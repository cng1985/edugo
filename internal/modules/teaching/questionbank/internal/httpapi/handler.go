package httpapi

import (
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/api"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(_, authed *gin.RouterGroup) {
	authed.POST("/question/create", h.create)
	authed.POST("/question/update", h.update)
	authed.POST("/question/detail", h.detail)
	authed.POST("/question/list", h.list)
	authed.POST("/question/submitReview", h.review)
	authed.POST("/question/approve", h.approve)
	authed.POST("/question/reject", h.reject)
	authed.POST("/aiQuestion/generate", h.generate)
	authed.POST("/aiQuestion/generateExercise", h.generate)
}

func (h *Handler) create(c *gin.Context) {
	var req api.WriteRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), req)
	write(c, out, err)
}
func (h *Handler) update(c *gin.Context) {
	var req api.WriteRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), req)
	write(c, out, err)
}
func (h *Handler) detail(c *gin.Context) {
	var req api.IDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Detail(c.Request.Context(), req.QuestionID)
	write(c, out, err)
}
func (h *Handler) list(c *gin.Context) {
	var req api.ListRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, total, page, size, err := h.svc.List(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.Page(c, out, page, size, total)
}
func (h *Handler) review(c *gin.Context)  { h.id(c) }
func (h *Handler) approve(c *gin.Context) { h.id(c) }
func (h *Handler) reject(c *gin.Context)  { h.id(c) }

func (h *Handler) id(c *gin.Context) {
	var req api.IDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	var out api.Question
	var err error
	switch c.FullPath() {
	case "/api/v1/question/approve":
		out, err = h.svc.Approve(c.Request.Context(), req)
	case "/api/v1/question/reject":
		out, err = h.svc.Reject(c.Request.Context(), req)
	default:
		out, err = h.svc.SubmitReview(c.Request.Context(), req)
	}
	write(c, out, err)
}

func (h *Handler) generate(c *gin.Context) {
	var req api.GenerateRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Generate(c.Request.Context(), req)
	write(c, out, err)
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
