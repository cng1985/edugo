package httpapi

import (
	"github.com/cng1985/edugo/internal/modules/teaching/course/api"
	"github.com/cng1985/edugo/internal/modules/teaching/course/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(public, authed *gin.RouterGroup) {
	authed.POST("/course/create", h.create)
	authed.POST("/course/update", h.update)
	authed.POST("/course/detail", h.detail)
	authed.POST("/course/list", h.list)
	authed.POST("/course/publish", h.publish)
	authed.POST("/course/archive", h.archive)
	authed.POST("/course/withdraw", h.withdraw)
	authed.POST("/course/generateSummary", h.summary)
	authed.POST("/chapter/create", h.createChapter)
	authed.POST("/chapter/update", h.updateChapter)
	authed.POST("/chapter/delete", h.deleteChapter)
	authed.POST("/chapter/list", h.detail)
	authed.POST("/chapter/detail", h.detail)
	authed.POST("/lesson/create", h.createLesson)
	authed.POST("/lesson/update", h.updateLesson)
	authed.POST("/lesson/delete", h.deleteLesson)
	authed.POST("/lesson/list", h.detail)
	authed.POST("/lesson/detail", h.detail)
	authed.POST("/aiQuestion/generateSummary", h.summary)
}

func (h *Handler) create(c *gin.Context) {
	var req api.CreateCourseRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateCourse(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) update(c *gin.Context) {
	var req api.UpdateCourseRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateCourse(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) detail(c *gin.Context) {
	var req api.CourseIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Detail(c.Request.Context(), req.CourseID)
	write(c, out, err)
}

func (h *Handler) list(c *gin.Context) {
	var req api.ListCourseRequest
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

func (h *Handler) publish(c *gin.Context) {
	var req api.CourseIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Publish(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) archive(c *gin.Context) {
	var req api.CourseIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Archive(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) withdraw(c *gin.Context) {
	var req api.CourseIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Withdraw(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) summary(c *gin.Context) {
	var req api.SummaryRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.GenerateSummary(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) createChapter(c *gin.Context) {
	var req api.ChapterWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateChapter(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) updateChapter(c *gin.Context) {
	var req api.ChapterWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateChapter(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) deleteChapter(c *gin.Context) {
	var req api.ChapterWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.DeleteChapter(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) createLesson(c *gin.Context) {
	var req api.LessonWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateLesson(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) updateLesson(c *gin.Context) {
	var req api.LessonWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateLesson(c.Request.Context(), req)
	write(c, out, err)
}

func (h *Handler) deleteLesson(c *gin.Context) {
	var req api.LessonWrite
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.DeleteLesson(c.Request.Context(), req)
	write(c, out, err)
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
