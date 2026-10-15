package httpapi

import (
	"encoding/json"

	"github.com/cng1985/edugo/internal/modules/assessment/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(_, authed *gin.RouterGroup) {
	authed.POST("/paper/create", h.createPaper)
	authed.POST("/paper/update", h.updatePaper)
	authed.POST("/paper/publish", h.publishPaper)
	authed.POST("/exam/create", h.createExam)
	authed.POST("/exam/publish", h.publishExam)
	authed.POST("/exam/list", h.listExams)
	authed.POST("/exam/start", h.start)
	authed.POST("/exam/saveAnswer", h.save)
	authed.POST("/exam/submit", h.submit)
	authed.POST("/exam/result", h.result)
	authed.POST("/exam/reviewScore", h.review)
	authed.POST("/exam/publishResult", h.publishResult)
}

func (h *Handler) createPaper(c *gin.Context) {
	var req struct {
		Title string `json:"title"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreatePaper(c.Request.Context(), req.Title)
	write(c, out, err)
}

func (h *Handler) updatePaper(c *gin.Context) {
	var req struct {
		PaperID            string   `json:"paperId"`
		Title              string   `json:"title"`
		ExpectedVersion    int64    `json:"expectedVersion"`
		QuestionVersionIDs []string `json:"questionVersionIds"`
		Scores             []int    `json:"scores"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdatePaper(c.Request.Context(), core.PaperInput{
		PaperID: req.PaperID, Title: req.Title, ExpectedVersion: req.ExpectedVersion, QuestionVersionIDs: req.QuestionVersionIDs, Scores: req.Scores,
	})
	write(c, out, err)
}

func (h *Handler) publishPaper(c *gin.Context) {
	var req struct {
		PaperID         string `json:"paperId"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.PublishPaper(c.Request.Context(), req.PaperID, req.ExpectedVersion)
	write(c, out, err)
}

func (h *Handler) createExam(c *gin.Context) {
	var req struct {
		Title         string `json:"title"`
		PaperID       string `json:"paperId"`
		DurationSec   int    `json:"durationSec"`
		MaxAttempts   int    `json:"maxAttempts"`
		PassScoreCenti int   `json:"passScoreCenti"`
		PublishMode   string `json:"publishMode"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateExam(c.Request.Context(), req.Title, req.PaperID, req.DurationSec, req.MaxAttempts, req.PassScoreCenti, req.PublishMode)
	write(c, out, err)
}

func (h *Handler) publishExam(c *gin.Context) {
	var req struct {
		ExamID          string `json:"examId"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.PublishExam(c.Request.Context(), req.ExamID, req.ExpectedVersion)
	write(c, out, err)
}

func (h *Handler) listExams(c *gin.Context) {
	out, err := h.svc.ListExams(c.Request.Context())
	write(c, out, err)
}

func (h *Handler) start(c *gin.Context) {
	var req struct {
		ExamID string `json:"examId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Start(c.Request.Context(), req.ExamID, c.GetHeader("Idempotency-Key"))
	write(c, out, err)
}

func (h *Handler) save(c *gin.Context) {
	var req struct {
		AttemptID       string          `json:"attemptId"`
		QuestionID      string          `json:"questionId"`
		ExpectedVersion int64           `json:"expectedVersion"`
		Answer          json.RawMessage `json:"answer"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.SaveAnswer(c.Request.Context(), req.AttemptID, req.QuestionID, req.ExpectedVersion, req.Answer)
	write(c, out, err)
}

func (h *Handler) submit(c *gin.Context) {
	var req struct {
		AttemptID string `json:"attemptId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Submit(c.Request.Context(), req.AttemptID)
	write(c, out, err)
}

func (h *Handler) result(c *gin.Context) {
	var req struct {
		AttemptID string `json:"attemptId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Result(c.Request.Context(), req.AttemptID)
	write(c, out, err)
}

func (h *Handler) review(c *gin.Context) {
	var req struct {
		AttemptID  string  `json:"attemptId"`
		QuestionID string  `json:"questionId"`
		Score      float64 `json:"score"`
		Reason     string  `json:"reason"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.ReviewScore(c.Request.Context(), req.AttemptID, req.QuestionID, req.Score, req.Reason)
	write(c, out, err)
}

func (h *Handler) publishResult(c *gin.Context) {
	var req struct {
		AttemptID string `json:"attemptId"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.PublishResult(c.Request.Context(), req.AttemptID)
	write(c, out, err)
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}
