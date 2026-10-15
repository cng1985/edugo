package httpapi

import (
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(_, authed *gin.RouterGroup) {
	authed.POST("/reviewCard/create", h.create)
	authed.POST("/reviewCard/due", h.due)
	authed.POST("/reviewCard/submit", h.submit)
	authed.POST("/reviewCard/stats", h.stats)
	authed.POST("/reviewCard/pause", h.pause)
	authed.POST("/reviewCard/resume", h.resume)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Front string `json:"front"`
		Back  string `json:"back"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), req.Front, req.Back)
	write(c, out, err)
}
func (h *Handler) due(c *gin.Context) {
	out, err := h.svc.Due(c.Request.Context())
	write(c, out, err)
}
func (h *Handler) submit(c *gin.Context) {
	var req struct {
		CardID          string `json:"cardId"`
		SubmissionID    string `json:"submissionId"`
		Quality         int    `json:"quality"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Submit(c.Request.Context(), req.CardID, req.SubmissionID, req.Quality, req.ExpectedVersion)
	write(c, out, err)
}
func (h *Handler) stats(c *gin.Context) {
	out, err := h.svc.Stats(c.Request.Context())
	write(c, out, err)
}
func (h *Handler) pause(c *gin.Context)  { h.status(c, true) }
func (h *Handler) resume(c *gin.Context) { h.status(c, false) }
func (h *Handler) status(c *gin.Context, pause bool) {
	var req struct {
		CardID          string `json:"cardId"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	var err error
	if pause {
		err = h.svc.Pause(c.Request.Context(), req.CardID, req.ExpectedVersion)
	} else {
		err = h.svc.Resume(c.Request.Context(), req.CardID, req.ExpectedVersion)
	}
	write(c, gin.H{"ok": err == nil}, err)
}
func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
