package organization

import (
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Org struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	Name      string `gorm:"size:200" json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Org) TableName() string { return "organizations" }

type Class struct {
	ID    string `gorm:"primaryKey;size:36" json:"id"`
	OrgID string `gorm:"size:36;index" json:"orgId"`
	Name  string `json:"name"`
}

func (Class) TableName() string { return "classes" }

type Member struct {
	ID     string `gorm:"primaryKey;size:36" json:"id"`
	OrgID  string `gorm:"size:36;uniqueIndex:uk_org_user" json:"orgId"`
	UserID string `gorm:"size:36;uniqueIndex:uk_org_user" json:"userId"`
	Role   string `gorm:"size:32" json:"role"`
}

func (Member) TableName() string { return "organization_members" }

type ClassMember struct {
	ID      string `gorm:"primaryKey;size:36"`
	ClassID string `gorm:"size:36;uniqueIndex:uk_class_user"`
	UserID  string `gorm:"size:36;uniqueIndex:uk_class_user"`
}

func (ClassMember) TableName() string { return "class_members" }

type Service struct {
	db  *gorm.DB
	cfg *config.Config
}

func New(db *gorm.DB, cfg *config.Config) *Service { return &Service{db: db, cfg: cfg} }

func (s *Service) Create(ctxName string, actor principal.Principal, name string) (Org, error) {
	if !s.cfg.Organization.Enabled {
		return Org{}, apperr.Unavailable("ORG_DISABLED", "组织扩展未启用")
	}
	if !actor.Can("org.manage") {
		return Org{}, apperr.Forbidden("无权管理机构")
	}
	row := Org{ID: xid.New(), Name: name, CreatedAt: time.Now().UTC()}
	_ = ctxName
	return row, s.db.Create(&row).Error
}

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{s} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/organization/create", func(c *gin.Context) {
		p, err := principal.Must(c.Request.Context())
		if err != nil {
			httpserver.Fail(c, err)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Create("", p, req.Name)
		if err != nil {
			httpserver.Fail(c, err)
			return
		}
		httpserver.OK(c, out)
	})
	g.POST("/organization/addMember", func(c *gin.Context) {
		if !h.svc.cfg.Organization.Enabled {
			httpserver.Fail(c, apperr.Unavailable("ORG_DISABLED", "组织扩展未启用"))
			return
		}
		var req struct {
			OrgID  string `json:"orgId"`
			UserID string `json:"userId"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		row := Member{ID: xid.New(), OrgID: req.OrgID, UserID: req.UserID, Role: "member"}
		if err := h.svc.db.Create(&row).Error; err != nil {
			httpserver.Fail(c, err)
			return
		}
		httpserver.OK(c, row)
	})
}

var Module = fx.Module("organization",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: []any{&Org{}, &Class{}, &Member{}, &ClassMember{}}}
		}, fx.ResultTags(`group:"schemas"`)),
	),
)
