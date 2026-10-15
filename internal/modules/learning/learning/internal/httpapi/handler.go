package httpapi

import (
	"github.com/cng1985/edugo/internal/modules/learning/learning/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(_, authed *gin.RouterGroup) {
	authed.POST("/enrollment/join", h.join)
	authed.POST("/enrollment/list", h.list)
	authed.POST("/enrollment/withdraw", h.withdraw)
	authed.POST("/studyRecord/startSession", h.start)
	authed.POST("/studyRecord/submit", h.submit)
	authed.POST("/progress/get", h.progress)
	authed.POST("/progress/statistics", h.progress)
}

func (h *Handler) join(c *gin.Context) {
	var req struct {
		CourseID string `json:"courseId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Join(c.Request.Context(), req.CourseID)
	write(c, out, err)
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context())
	write(c, out, err)
}

func (h *Handler) withdraw(c *gin.Context) {
	var req struct {
		CourseID string `json:"courseId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	err := h.svc.Withdraw(c.Request.Context(), req.CourseID)
	write(c, gin.H{"withdrawn": err == nil}, err)
}

func (h *Handler) start(c *gin.Context) {
	var req struct {
		LessonID string `json:"lessonId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.StartSession(c.Request.Context(), req.LessonID)
	write(c, out, err)
}

func (h *Handler) submit(c *gin.Context) {
	var req struct {
		SessionID  string `json:"sessionId"`
		Sequence   int    `json:"sequence"`
		RecordType string `json:"recordType"`
		PositionMs int64  `json:"positionMs"`
		Passed     bool   `json:"passed"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Submit(c.Request.Context(), core.SubmitInput{
		SessionID: req.SessionID, Sequence: req.Sequence, RecordType: req.RecordType, PositionMs: req.PositionMs, Passed: req.Passed,
	})
	write(c, out, err)
}

func (h *Handler) progress(c *gin.Context) {
	var req struct {
		CourseID string `json:"courseId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Progress(c.Request.Context(), req.CourseID)
	write(c, out, err)
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
