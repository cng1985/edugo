package llm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Usage struct {
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	Known            bool
}

type ChatRequest struct {
	Module        string
	TaskID        string
	Profile       string
	PromptVersion string
	SchemaVersion string
	System        string
	User          string
	JSONMode      bool
}

type ChatResult struct {
	Content    string
	Usage      Usage
	UpstreamID string
	Model      string
}

type ChatStream interface {
	Recv(ctx context.Context) (delta string, done bool, usage Usage, err error)
	Close() error
}

type Client interface {
	Enabled() bool
	Complete(ctx context.Context, req ChatRequest) (ChatResult, error)
	Stream(ctx context.Context, req ChatRequest) (ChatStream, error)
	Embed(ctx context.Context, module, taskID, text string) ([]float64, error)
}

type CallLog struct {
	ID               string `gorm:"primaryKey;size:36"`
	Module           string `gorm:"size:64;index"`
	TaskID           string `gorm:"size:64;index"`
	Profile          string `gorm:"size:64"`
	Model            string `gorm:"size:128"`
	PromptVersion    string `gorm:"size:64"`
	SchemaVersion    string `gorm:"size:64"`
	DurationMs       int64
	Status           string `gorm:"size:32"`
	Retries          int
	UpstreamID       string `gorm:"size:128"`
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	UsageKnown       bool
	ErrorCode        string `gorm:"size:64"`
	CreatedAt        time.Time
}

func (CallLog) TableName() string { return "llm_call_logs" }

func Models() []any { return []any{&CallLog{}} }

type Gateway struct {
	cfg   *config.Config
	db    *gorm.DB
	inner *openAI
}

func New(cfg *config.Config, db *gorm.DB) Client {
	return &Gateway{cfg: cfg, db: db, inner: newOpenAI(cfg)}
}

func (g *Gateway) Enabled() bool { return g.cfg.AI.Enabled }

func (g *Gateway) Complete(ctx context.Context, req ChatRequest) (ChatResult, error) {
	if !g.cfg.AI.Enabled {
		return ChatResult{}, apperr.Unavailable("AI_DISABLED", "AI 能力未启用")
	}
	profile, ok := g.cfg.Profile(req.Profile)
	if !ok {
		return ChatResult{}, apperr.Unavailable("AI_PROFILE_UNAVAILABLE", "模型配置不可用")
	}
	start := time.Now()
	res, retries, err := g.inner.complete(ctx, profile, req)
	g.writeLog(req, profile.Model, res, retries, time.Since(start), err)
	return res, err
}

func (g *Gateway) Stream(ctx context.Context, req ChatRequest) (ChatStream, error) {
	if !g.cfg.AI.Enabled {
		return nil, apperr.Unavailable("AI_DISABLED", "AI 能力未启用")
	}
	profile, ok := g.cfg.Profile(req.Profile)
	if !ok {
		return nil, apperr.Unavailable("AI_PROFILE_UNAVAILABLE", "模型配置不可用")
	}
	stream, err := g.inner.stream(ctx, profile, req)
	if err != nil {
		g.writeLog(req, profile.Model, ChatResult{}, 0, 0, err)
		return nil, err
	}
	return &loggedStream{inner: stream, gw: g, req: req, model: profile.Model, start: time.Now()}, nil
}

func (g *Gateway) Embed(ctx context.Context, module, taskID, text string) ([]float64, error) {
	if !g.cfg.AI.Enabled {
		return nil, apperr.Unavailable("AI_DISABLED", "AI 能力未启用")
	}
	profile, ok := g.cfg.Profile("embedding")
	if !ok {
		return nil, apperr.Unavailable("EMBEDDING_DISABLED", "未配置向量模型，当前检索为关键词降级")
	}
	vec, upstream, err := g.inner.embed(ctx, profile, text)
	status := "succeeded"
	code := ""
	if err != nil {
		status = "failed"
		code = apperr.As(err).ErrorCode
	}
	_ = g.db.Create(&CallLog{
		ID: xid.New(), Module: module, TaskID: taskID, Profile: "embedding", Model: profile.Model,
		Status: status, UpstreamID: upstream, ErrorCode: code, UsageKnown: false, CreatedAt: time.Now().UTC(),
	}).Error
	return vec, err
}

func (g *Gateway) writeLog(req ChatRequest, model string, res ChatResult, retries int, d time.Duration, err error) {
	status := "succeeded"
	code := ""
	if err != nil {
		status = "failed"
		code = apperr.As(err).ErrorCode
		if errors.Is(err, context.Canceled) {
			code = "CANCELLED"
		}
	}
	row := CallLog{
		ID: xid.New(), Module: req.Module, TaskID: req.TaskID, Profile: req.Profile, Model: model,
		PromptVersion: req.PromptVersion, SchemaVersion: req.SchemaVersion, DurationMs: d.Milliseconds(),
		Status: status, Retries: retries, UpstreamID: res.UpstreamID, UsageKnown: res.Usage.Known,
		PromptTokens: res.Usage.PromptTokens, CompletionTokens: res.Usage.CompletionTokens, TotalTokens: res.Usage.TotalTokens,
		ErrorCode: code, CreatedAt: time.Now().UTC(),
	}
	_ = g.db.Create(&row).Error
}

type loggedStream struct {
	inner  ChatStream
	gw     *Gateway
	req    ChatRequest
	model  string
	start  time.Time
	logged bool
	usage  Usage
	upID   string
}

func (s *loggedStream) Recv(ctx context.Context) (string, bool, Usage, error) {
	delta, done, usage, err := s.inner.Recv(ctx)
	if usage.Known {
		s.usage = usage
	}
	if done || err != nil {
		s.finish(err)
	}
	return delta, done, usage, err
}

func (s *loggedStream) Close() error { return s.inner.Close() }

func (s *loggedStream) finish(err error) {
	if s.logged {
		return
	}
	s.logged = true
	s.gw.writeLog(s.req, s.model, ChatResult{Usage: s.usage, UpstreamID: s.upID}, 0, time.Since(s.start), err)
}

func intPtr(v int) *int { return &v }

func clampText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
