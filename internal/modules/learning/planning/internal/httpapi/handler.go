package httpapi

import (
	"time"

	"github.com/cng1985/edugo/internal/modules/learning/planning/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/studyPlan/create", h.create)
	g.POST("/studyPlan/list", h.list)
	g.POST("/studyPlan/start", func(c *gin.Context) { h.status(c, "active") })
	g.POST("/studyPlan/pause", func(c *gin.Context) { h.status(c, "paused") })
	g.POST("/studyPlan/resume", func(c *gin.Context) { h.status(c, "active") })
	g.POST("/studyPlan/archive", func(c *gin.Context) { h.status(c, "archived") })
	g.POST("/studyPlan/detail", h.detailPlan)
	g.POST("/studyPlan/update", h.detailPlan)
	g.POST("/learningPath/create", h.path)
	g.POST("/learningPath/list", h.listPaths)
	g.POST("/learningPath/detail", h.pathDetail)
	g.POST("/learningPath/activate", h.activate)
	g.POST("/learningPath/generate", h.generate)
	g.POST("/aiSuggestion/generatePath", h.generate)
	g.POST("/learningPath/progress", h.pathDetail)
	g.POST("/learningPath/reorder", h.pathDetail)
	g.POST("/learningPath/delete", h.archivePath)
	g.POST("/learningPath/update", h.pathDetail)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Title    string `json:"title"`
		Deadline string `json:"deadlineAt"`
		Items    []struct {
			Title    string `json:"title"`
			Required bool   `json:"required"`
			Manual   bool   `json:"manual"`
		} `json:"items"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	deadline, err := time.Parse(time.RFC3339, req.Deadline)
	if err != nil {
		httpserver.Fail(c, apperr.Invalid("截止时间需使用 RFC3339"))
		return
	}
	items := make([]core.Item, len(req.Items))
	for i, item := range req.Items {
		items[i] = core.Item{Title: item.Title, Required: item.Required, Manual: item.Manual}
	}
	out, err := h.svc.CreatePlan(c.Request.Context(), req.Title, deadline, items)
	write(c, out, err)
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.ListPlans(c.Request.Context())
	write(c, out, err)
}

func (h *Handler) status(c *gin.Context, status string) {
	var req struct {
		PlanID string `json:"planId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.SetStatus(c.Request.Context(), req.PlanID, status)
	write(c, out, err)
}

func (h *Handler) detailPlan(c *gin.Context) { h.list(c) }

func (h *Handler) path(c *gin.Context) {
	var req struct {
		Title string `json:"title"`
		Nodes []struct {
			Title    string `json:"title"`
			CourseID string `json:"courseId"`
			Required bool   `json:"required"`
		} `json:"nodes"`
		Edges [][2]int `json:"edges"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	nodes := make([]core.Node, len(req.Nodes))
	for i, n := range req.Nodes {
		nodes[i] = core.Node{Title: n.Title, CourseID: n.CourseID, Required: n.Required, SortOrder: i}
	}
	out, err := h.svc.CreatePath(c.Request.Context(), req.Title, nodes, req.Edges)
	write(c, out, err)
}

func (h *Handler) pathDetail(c *gin.Context) {
	var req struct {
		PathID string `json:"pathId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Detail(c.Request.Context(), req.PathID)
	write(c, out, err)
}

func (h *Handler) activate(c *gin.Context) {
	var req struct {
		PathID string `json:"pathId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Activate(c.Request.Context(), req.PathID)
	write(c, out, err)
}

func (h *Handler) generate(c *gin.Context) {
	var req struct {
		Goal string `json:"goal"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Generate(c.Request.Context(), req.Goal)
	write(c, out, err)
}

func (h *Handler) listPaths(c *gin.Context) {
	out, err := h.svc.ListPaths(c.Request.Context())
	write(c, out, err)
}

func (h *Handler) archivePath(c *gin.Context) {
	var req struct {
		PathID string `json:"pathId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	err := h.svc.ArchivePath(c.Request.Context(), req.PathID)
	write(c, gin.H{"archived": err == nil}, err)
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
