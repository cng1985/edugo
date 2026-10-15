package query

import (
	"reflect"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type engineIn struct {
	fx.In
	DB     *gorm.DB
	Cfg    *config.Config
	Models []Model `group:"queries"`
}

func NewEngine(in engineIn) *Engine { return New(in.DB, in.Cfg, in.Models) }

type Handler struct{ engine *Engine }

func NewHandler(engine *Engine) *Handler { return &Handler{engine: engine} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/query/search", func(c *gin.Context) { h.run(c, "search") })
	g.POST("/query/list", func(c *gin.Context) { h.run(c, "list") })
	g.POST("/query/one", func(c *gin.Context) { h.run(c, "one") })
}

func (h *Handler) run(c *gin.Context, kind string) {
	p, err := principal.Must(c.Request.Context())
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	var req Request
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	data, total, page, size, err := h.engine.Run(c.Request.Context(), kind, p, req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	if kind == "one" {
		httpserver.One(c, firstOrNil(data))
		return
	}
	httpserver.Page(c, data, page, size, total)
}

func firstOrNil(data any) any {
	if data == nil {
		return nil
	}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice {
		if v.Len() == 0 {
			return nil
		}
		return v.Index(0).Interface()
	}
	return data
}

var Module = fx.Module("query-http",
	fx.Provide(NewEngine, NewHandler),
	fx.Provide(fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`))),
)
