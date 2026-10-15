package analytics

import (
	"context"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Projection struct {
	ID        string `gorm:"primaryKey;size:36"`
	UserID    string `gorm:"size:36;uniqueIndex:uk_metric"`
	Metric    string `gorm:"size:64;uniqueIndex:uk_metric"`
	Bucket    string `gorm:"size:64;uniqueIndex:uk_metric"`
	Value     int64
	DataAsOf  time.Time
	UpdatedAt time.Time
}

func (Projection) TableName() string { return "learning_metric_projections" }

type Suggestion struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	UserID    string `gorm:"size:36;index" json:"userId"`
	Content   string `gorm:"type:text" json:"content"`
	DataAsOf  time.Time `json:"dataAsOf"`
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Suggestion) TableName() string { return "ai_suggestions" }

type Report struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	UserID    string `gorm:"size:36;index" json:"userId"`
	Title     string `json:"title"`
	Body      string `gorm:"type:text" json:"body"`
	DataAsOf  time.Time `json:"dataAsOf"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Report) TableName() string { return "learning_reports" }

type ReportJob struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	UserID    string `gorm:"size:36;index" json:"userId"`
	Status    string `gorm:"size:32" json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

func (ReportJob) TableName() string { return "report_jobs" }

type Service struct {
	db  *gorm.DB
	llm llm.Client
}

func New(db *gorm.DB, client llm.Client) *Service { return &Service{db: db, llm: client} }

func (s *Service) Add(userID, metric string, delta int64) error {
	var row Projection
	err := s.db.Where("user_id = ? AND metric = ? AND bucket = ?", userID, metric, "all").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		row = Projection{ID: xid.New(), UserID: userID, Metric: metric, Bucket: "all", Value: delta, DataAsOf: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		return s.db.Create(&row).Error
	}
	if err != nil {
		return err
	}
	row.Value += delta
	row.DataAsOf = time.Now().UTC()
	return s.db.Save(&row).Error
}

func (s *Service) Generate(ctx context.Context) (Suggestion, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Suggestion{}, err
	}
	text := "根据截至当前的学习记录，建议优先完成未完成的必修课时，并复习到期卡片。该建议不会改变计划、成绩或证书。"
	if s.llm.Enabled() {
		res, err := s.llm.Complete(ctx, llm.ChatRequest{Module: "analytics", Profile: "suggestion", PromptVersion: "suggest-v1", User: "请给出学习建议", System: "不要修改任何业务状态。"})
		if err == nil && res.Content != "" {
			text = res.Content
		}
	}
	row := Suggestion{ID: xid.New(), UserID: p.UserID, Content: text, DataAsOf: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	return row, s.db.Create(&row).Error
}

func (s *Service) List(ctx context.Context) ([]Suggestion, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var rows []Suggestion
	err = s.db.Where("user_id = ?", p.UserID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) Read(ctx context.Context, id string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res := s.db.Model(&Suggestion{}).Where("id = ? AND user_id = ?", id, p.UserID).Update("read_at", now)
	if res.RowsAffected == 0 {
		return apperr.NotFound("建议不存在")
	}
	return res.Error
}

func (s *Service) CreateReport(ctx context.Context) (Report, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Report{}, err
	}
	row := Report{ID: xid.New(), UserID: p.UserID, Title: "学习报告", Body: "报告基于已提交的学习与测评事件，分母为 0 的指标显示为空。", DataAsOf: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	_ = s.db.Create(&ReportJob{ID: xid.New(), UserID: p.UserID, Status: "succeeded", CreatedAt: time.Now().UTC()}).Error
	return row, s.db.Create(&row).Error
}

func (s *Service) Reports(ctx context.Context) ([]Report, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var rows []Report
	err = s.db.Where("user_id = ?", p.UserID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{s} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/aiSuggestion/generate", func(c *gin.Context) {
		out, err := h.svc.Generate(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/aiSuggestion/list", func(c *gin.Context) {
		out, err := h.svc.List(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/aiSuggestion/read", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		write(c, gin.H{"read": true}, h.svc.Read(c.Request.Context(), req.ID))
	})
	g.POST("/learningReport/create", func(c *gin.Context) {
		out, err := h.svc.CreateReport(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/learningReport/list", func(c *gin.Context) {
		out, err := h.svc.Reports(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/learningReport/detail", func(c *gin.Context) {
		out, err := h.svc.Reports(c.Request.Context())
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

var Module = fx.Module("analytics",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: []any{&Projection{}, &Suggestion{}, &Report{}, &ReportJob{}}}
		}, fx.ResultTags(`group:"schemas"`)),
	),
)
