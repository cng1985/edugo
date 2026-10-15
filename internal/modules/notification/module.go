package notification

import (
	"context"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Message struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	UserID    string `gorm:"size:36;index:uk_notice,unique;not null" json:"userId"`
	DedupKey  string `gorm:"size:128;index:uk_notice,unique;not null" json:"dedupKey"`
	Title     string `json:"title"`
	Body      string `gorm:"type:text" json:"body"`
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Message) TableName() string { return "notifications" }

type Preference struct {
	UserID string `gorm:"primaryKey;size:36" json:"userId"`
	Review bool   `json:"review"`
	Plan   bool   `json:"plan"`
	Score  bool   `json:"score"`
	Cert   bool   `json:"cert"`
}

func (Preference) TableName() string { return "notification_preferences" }

type Delivery struct {
	ID        string `gorm:"primaryKey;size:36"`
	MessageID string `gorm:"size:36;index"`
	Channel   string `gorm:"size:32"`
	Status    string `gorm:"size:32"`
	Error     string `gorm:"size:512"`
	CreatedAt time.Time
}

func (Delivery) TableName() string { return "notification_deliveries" }

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Notify(userID, dedup, title, body string) error {
	row := Message{ID: xid.New(), UserID: userID, DedupKey: dedup, Title: title, Body: body, CreatedAt: time.Now().UTC()}
	err := s.db.Create(&row).Error
	if err != nil {
		var existing Message
		if e := s.db.Where("user_id = ? AND dedup_key = ?", userID, dedup).First(&existing).Error; e == nil {
			return nil
		}
	}
	if err == nil {
		_ = s.db.Create(&Delivery{ID: xid.New(), MessageID: row.ID, Channel: "inbox", Status: "sent", CreatedAt: time.Now().UTC()}).Error
	}
	return err
}

func (s *Service) List(ctx context.Context) ([]Message, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var rows []Message
	err = s.db.Where("user_id = ?", p.UserID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) Read(ctx context.Context, id string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res := s.db.Model(&Message{}).Where("id = ? AND user_id = ?", id, p.UserID).Update("read_at", now)
	if res.RowsAffected == 0 {
		return apperr.NotFound("消息不存在")
	}
	return res.Error
}

func (s *Service) Preferences(ctx context.Context) (Preference, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Preference{}, err
	}
	var row Preference
	err = s.db.First(&row, "user_id = ?", p.UserID).Error
	if err == gorm.ErrRecordNotFound {
		row = Preference{UserID: p.UserID, Review: true, Plan: true, Score: true, Cert: true}
		_ = s.db.Create(&row).Error
		return row, nil
	}
	return row, err
}

func (s *Service) UpdatePreferences(ctx context.Context, row Preference) (Preference, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Preference{}, err
	}
	row.UserID = p.UserID
	return row, s.db.Save(&row).Error
}

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/notification/list", func(c *gin.Context) {
		out, err := h.svc.List(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/notification/read", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		write(c, gin.H{"read": true}, h.svc.Read(c.Request.Context(), req.ID))
	})
	g.POST("/notification/preferences", func(c *gin.Context) {
		out, err := h.svc.Preferences(c.Request.Context())
		write(c, out, err)
	})
	g.POST("/notification/updatePreferences", func(c *gin.Context) {
		var req Preference
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.UpdatePreferences(c.Request.Context(), req)
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

var Module = fx.Module("notification",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(
		New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: []any{&Message{}, &Preference{}, &Delivery{}}}
		}, fx.ResultTags(`group:"schemas"`)),
	),
)
