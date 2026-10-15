package credential

import (
	"context"
	"time"

	learnapi "github.com/cng1985/edugo/internal/modules/learning/learning/api"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Rule struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	CourseID  string `gorm:"size:36;index" json:"courseId"`
	Version   int    `json:"version"`
	Title     string `json:"title"`
	Status    string `gorm:"size:32" json:"status"`
}

func (Rule) TableName() string { return "certificate_rules" }

type Template struct {
	ID    string `gorm:"primaryKey;size:36" json:"id"`
	Name  string `json:"name"`
	Body  string `gorm:"type:text" json:"body"`
}

func (Template) TableName() string { return "certificate_templates" }

type Certificate struct {
	ID               string `gorm:"primaryKey;size:36" json:"id"`
	UserID           string `gorm:"size:36;not null;uniqueIndex:uk_cert" json:"userId"`
	QualificationKey string `gorm:"size:160;not null;uniqueIndex:uk_cert" json:"qualificationKey"`
	RuleVersion      int    `json:"ruleVersion"`
	CertificateNo    string `gorm:"size:64;uniqueIndex" json:"certificateNo"`
	Status           string `gorm:"size:32" json:"status"`
	Evidence         string `gorm:"type:text" json:"evidence"`
	Title            string `json:"title"`
	IssuedAt         time.Time `json:"issuedAt"`
	RevokedAt        *time.Time `json:"revokedAt"`
	RevokeReason     string `json:"revokeReason"`
}

func (Certificate) TableName() string { return "certificates" }

type EventRow struct {
	ID            string `gorm:"primaryKey;size:36"`
	CertificateID string `gorm:"size:36;index"`
	Action        string `gorm:"size:32"`
	ActorID       string `gorm:"size:36"`
	CreatedAt     time.Time
}

func (EventRow) TableName() string { return "certificate_events" }

type Service struct {
	db      *gorm.DB
	learning learnapi.Reader
}

func New(db *gorm.DB, learning learnapi.Reader) *Service {
	return &Service{db: db, learning: learning}
}

func (s *Service) Issue(ctx context.Context, userID, courseID, evidence string) (Certificate, error) {
	key := "course:" + courseID + ":rule:default:v1"
	var existing Certificate
	err := s.db.Where("user_id = ? AND qualification_key = ?", userID, key).First(&existing).Error
	if err == nil {
		return existing, nil
	}
	row := Certificate{
		ID: xid.New(), UserID: userID, QualificationKey: key, RuleVersion: 1, CertificateNo: "EDU-" + xid.New()[:8],
		Status: "issued", Evidence: evidence, Title: "课程完成证书", IssuedAt: time.Now().UTC(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if e := s.db.Where("user_id = ? AND qualification_key = ?", userID, key).First(&existing).Error; e == nil {
			return existing, nil
		}
		return Certificate{}, err
	}
	_ = s.db.Create(&EventRow{ID: xid.New(), CertificateID: row.ID, Action: "issued", CreatedAt: time.Now().UTC()}).Error
	ev, _ := eventbus.NewEvent(ctx, "CertificateIssued.v1", row.ID, 1, map[string]any{"certificateId": row.ID, "userId": userID, "qualificationKey": key, "issuedAt": row.IssuedAt})
	_ = eventbus.Insert(s.db, ev)
	return row, nil
}

func (s *Service) TryIssue(ctx context.Context, userID, courseID, evidence string) (Certificate, error) {
	done, evidenceID, err := s.learning.Completion(ctx, userID, courseID)
	if err != nil || !done {
		return Certificate{}, apperr.Invalid("尚未满足发证资格")
	}
	if evidence == "" {
		evidence = evidenceID
	}
	return s.Issue(ctx, userID, courseID, evidence)
}

func (s *Service) List(ctx context.Context) ([]Certificate, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	q := s.db.Model(&Certificate{})
	if !p.Can("certificate.issue") {
		q = q.Where("user_id = ?", p.UserID)
	}
	var rows []Certificate
	err = q.Order("issued_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) Detail(ctx context.Context, id string) (Certificate, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Certificate{}, err
	}
	var row Certificate
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		return Certificate{}, apperr.NotFound("证书不存在")
	}
	if row.UserID != p.UserID && !p.Can("certificate.issue") {
		return Certificate{}, apperr.NotFound("证书不存在")
	}
	return row, nil
}

func (s *Service) Revoke(ctx context.Context, id, reason string) (Certificate, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Certificate{}, err
	}
	if !p.Can("certificate.revoke") {
		return Certificate{}, apperr.Forbidden("无权撤销证书")
	}
	var row Certificate
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		return Certificate{}, apperr.NotFound("证书不存在")
	}
	now := time.Now().UTC()
	row.Status = "revoked"
	row.RevokedAt = &now
	row.RevokeReason = reason
	if err := s.db.Save(&row).Error; err != nil {
		return Certificate{}, err
	}
	_ = s.db.Create(&EventRow{ID: xid.New(), CertificateID: row.ID, Action: "revoked", ActorID: p.UserID, CreatedAt: now}).Error
	return row, nil
}

func (s *Service) Verify(no string) (map[string]any, error) {
	var row Certificate
	if err := s.db.Where("certificate_no = ?", no).First(&row).Error; err != nil {
		return map[string]any{"status": "not_found"}, nil
	}
	return map[string]any{"status": row.Status, "title": row.Title, "issuedAt": row.IssuedAt, "revokedAt": row.RevokedAt}, nil
}

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{s} }

func (h *Handler) Register(public, g *gin.RouterGroup) {
	g.POST("/certificate/list", func(c *gin.Context) {
		out, err := h.svc.List(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/certificate/detail", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Detail(c.Request.Context(), req.ID)
		write(c, out, err)
	})
	g.POST("/certificate/issue", func(c *gin.Context) {
		p, _ := principal.From(c.Request.Context())
		var req struct {
			CourseID string `json:"courseId"`
			UserID   string `json:"userId"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		userID := p.UserID
		if req.UserID != "" && p.Can("certificate.issue") {
			userID = req.UserID
		}
		out, err := h.svc.TryIssue(c.Request.Context(), userID, req.CourseID, "")
		write(c, out, err)
	})
	g.POST("/certificate/revoke", func(c *gin.Context) {
		var req struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Revoke(c.Request.Context(), req.ID, req.Reason)
		write(c, out, err)
	})
	public.POST("/certificate/verify", func(c *gin.Context) {
		var req struct {
			CertificateNo string `json:"certificateNo"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Verify(req.CertificateNo)
		write(c, out, err)
	})
	g.POST("/certificate/download", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		row, err := h.svc.Detail(c.Request.Context(), req.ID)
		if err != nil {
			httpserver.Fail(c, err)
			return
		}
		c.Header("Content-Disposition", "attachment; filename=certificate.txt")
		c.Data(200, "text/plain; charset=utf-8", []byte(row.Title+" "+row.CertificateNo+" "+row.Status))
	})
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}

var Module = fx.Module("credential",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: []any{&Rule{}, &Template{}, &Certificate{}, &EventRow{}}}
		}, fx.ResultTags(`group:"schemas"`)),
		fx.Annotate(func(s *Service) eventbus.Consumer {
			return eventbus.Consumer{
				Name:  "credential",
				Types: map[string]struct{}{learnapi.EventCourseCompleted: {}},
				Handle: func(ctx context.Context, tx *gorm.DB, ev eventbus.Event) error {
					var payload struct {
						UserID     string `json:"userId"`
						CourseID   string `json:"courseId"`
						EvidenceID string `json:"evidenceId"`
					}
					if err := jsonUnmarshal(ev.Payload, &payload); err != nil {
						return err
					}
					_, err := s.Issue(ctx, payload.UserID, payload.CourseID, payload.EvidenceID)
					return err
				},
			}
		}, fx.ResultTags(`group:"consumers"`)),
	),
)

func jsonUnmarshal(b []byte, dest any) error { return jsonUnmarshalStd(b, dest) }
