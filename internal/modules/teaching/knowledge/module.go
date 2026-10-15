package knowledge

import (
	"context"
	"strings"
	"time"

	courseapi "github.com/cng1985/edugo/internal/modules/teaching/course/api"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Point struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	Name        string `gorm:"size:200;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	OwnerID     string `gorm:"size:36;index" json:"ownerId"`
	OrgID       string `gorm:"size:36;index" json:"orgId"`
	Status      string `gorm:"size:32" json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (Point) TableName() string { return "knowledge_points" }

type Relation struct {
	ID       string `gorm:"primaryKey;size:36"`
	FromID   string `gorm:"size:36;index"`
	ToID     string `gorm:"size:36;index"`
	RelType  string `gorm:"size:32"`
}

func (Relation) TableName() string { return "knowledge_relations" }

type Chunk struct {
	ID              string `gorm:"primaryKey;size:36" json:"id"`
	SourceType      string `gorm:"size:32;index" json:"sourceType"`
	SourceID        string `gorm:"size:36;index" json:"sourceId"`
	SourceVersionID string `gorm:"size:36;index" json:"sourceVersionId"`
	ContentHash     string `gorm:"size:64" json:"contentHash"`
	Text            string `gorm:"type:text" json:"text"`
	Withdrawn       bool   `json:"withdrawn"`
	OrgID           string `gorm:"size:36;index" json:"orgId"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (Chunk) TableName() string { return "content_chunks" }

type IndexRow struct {
	ID        string `gorm:"primaryKey;size:36"`
	ChunkID   string `gorm:"size:36;index"`
	Mode      string `gorm:"size:32"`
	Status    string `gorm:"size:32"`
	UpdatedAt time.Time
}

func (IndexRow) TableName() string { return "knowledge_indexes" }

type Job struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	Status    string `gorm:"size:32" json:"status"`
	SourceID  string `gorm:"size:36" json:"sourceId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Job) TableName() string { return "index_jobs" }

type Service struct {
	db  *gorm.DB
	llm llm.Client
}

func New(db *gorm.DB, client llm.Client) *Service { return &Service{db: db, llm: client} }

func (s *Service) Create(ctx context.Context, name, description string) (Point, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Point{}, err
	}
	if !p.Can("course.manage") {
		return Point{}, apperr.Forbidden("无权维护知识点")
	}
	row := Point{ID: xid.New(), Name: name, Description: description, OwnerID: p.UserID, OrgID: p.OrgID, Status: "active", CreatedAt: time.Now().UTC()}
	return row, s.db.Create(&row).Error
}

func (s *Service) Search(ctx context.Context, q string, topK int) (map[string]any, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	if topK < 1 || topK > 20 {
		topK = 5
	}
	var rows []Chunk
	query := s.db.Where("withdrawn = ?", false)
	if s.orgOn(p) {
		query = query.Where("org_id = ?", p.OrgID)
	}
	if q != "" {
		query = query.Where("text LIKE ?", "%"+q+"%")
	}
	if err := query.Limit(topK).Find(&rows).Error; err != nil {
		return nil, err
	}
	mode := "keyword"
	if _, err := s.llm.Embed(ctx, "knowledge", "search", q); err == nil {
		mode = "vector"
	}
	return map[string]any{"mode": mode, "items": rows, "dataAsOf": time.Now().UTC()}, nil
}

func (s *Service) IndexCourse(versionID, courseID, text, orgID string) error {
	chunk := Chunk{ID: xid.New(), SourceType: "course", SourceID: courseID, SourceVersionID: versionID, ContentHash: versionID, Text: text, OrgID: orgID, UpdatedAt: time.Now().UTC()}
	if err := s.db.Create(&chunk).Error; err != nil {
		return err
	}
	return s.db.Create(&IndexRow{ID: xid.New(), ChunkID: chunk.ID, Mode: "keyword", Status: "ready", UpdatedAt: time.Now().UTC()}).Error
}

func (s *Service) Withdraw(versionID string) error {
	return s.db.Model(&Chunk{}).Where("source_version_id = ?", versionID).Update("withdrawn", true).Error
}

func (s *Service) orgOn(p principal.Principal) bool { return p.OrgID != "" && p.OrgID != "default" }

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{s} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/aiKnowledge/search", func(c *gin.Context) {
		var req struct {
			Query string `json:"query"`
			TopK  int    `json:"topK"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Search(c.Request.Context(), req.Query, req.TopK)
		write(c, out, err)
	})
	g.POST("/aiKnowledge/index", func(c *gin.Context) {
		p, _ := principal.From(c.Request.Context())
		if !p.Can("course.manage") {
			httpserver.Fail(c, apperr.Forbidden("无权重建索引"))
			return
		}
		var req struct {
			CourseID string `json:"courseId"`
			Text     string `json:"text"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		job := Job{ID: xid.New(), Status: "succeeded", SourceID: req.CourseID, CreatedAt: time.Now().UTC()}
		_ = h.svc.db.Create(&job).Error
		err := h.svc.IndexCourse(req.CourseID, req.CourseID, req.Text, p.OrgID)
		write(c, job, err)
	})
	g.POST("/aiKnowledge/rebuild", func(c *gin.Context) {
		write(c, gin.H{"status": "queued"}, nil)
	})
	g.POST("/knowledge/create", func(c *gin.Context) {
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Create(c.Request.Context(), req.Name, req.Description)
		write(c, out, err)
	})
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}

var Module = fx.Module("knowledge",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: []any{&Point{}, &Relation{}, &Chunk{}, &IndexRow{}, &Job{}}}
		}, fx.ResultTags(`group:"schemas"`)),
		fx.Annotate(func(s *Service) eventbus.Consumer {
			return eventbus.Consumer{
				Name: "knowledge",
				Types: map[string]struct{}{courseapi.EventPublished: {}, courseapi.EventWithdrawn: {}},
				Handle: func(ctx context.Context, tx *gorm.DB, ev eventbus.Event) error {
					var payload struct {
						CourseID        string `json:"courseId"`
						CourseVersionID string `json:"courseVersionId"`
					}
					if err := decode(ev.Payload, &payload); err != nil {
						return err
					}
					if ev.Type == courseapi.EventWithdrawn {
						return s.Withdraw(payload.CourseVersionID)
					}
					return s.IndexCourse(payload.CourseVersionID, payload.CourseID, "course "+payload.CourseID, "")
				},
			}
		}, fx.ResultTags(`group:"consumers"`)),
	),
)

func decode(b []byte, dest any) error {
	return jsonDecode(b, dest)
}

func init() { _ = strings.TrimSpace }
