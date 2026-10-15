package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cng1985/edugo/internal/platform/authn"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/trace"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
)

type SessionResolver interface {
	ResolveSession(ctx context.Context, userID, sessionID string) (principal.Principal, error)
}

type AgentResolver interface {
	ResolveAgent(ctx context.Context, token string) (principal.Principal, error)
}

func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := xid.New()
		ctx := trace.With(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Trace-Id", id)
		c.Next()
	}
}

func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			"time", time.Now().UTC().Format(time.RFC3339),
			"module", "httpserver",
			"action", c.FullPath(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latencyMs", time.Since(start).Milliseconds(),
			"traceId", trace.From(c.Request.Context()),
		)
	}
}

func Recover(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", "traceId", trace.From(c.Request.Context()), "err", "panic")
				Fail(c, apperr.Internal("内部错误"))
				c.Abort()
			}
		}()
		c.Next()
	}
}

func LimitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && strings.HasPrefix(c.Request.URL.Path, "/api/") && !strings.Contains(c.Request.URL.Path, "/file/upload") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}

func Authenticate(cfg *config.Config, sessions SessionResolver, agents AgentResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			Fail(c, apperr.Unauthorized("未登录"))
			c.Abort()
			return
		}
		var p principal.Principal
		var err error
		switch {
		case strings.HasPrefix(header, "Bearer "):
			uid, sid, parseErr := authn.ParseAccess(cfg.Auth.JWTSecret, strings.TrimPrefix(header, "Bearer "))
			if parseErr != nil {
				Fail(c, parseErr)
				c.Abort()
				return
			}
			p, err = sessions.ResolveSession(c.Request.Context(), uid, sid)
		case strings.HasPrefix(header, "Agent "):
			p, err = agents.ResolveAgent(c.Request.Context(), strings.TrimPrefix(header, "Agent "))
		default:
			err = apperr.Unauthorized("认证方案无法识别")
		}
		if err != nil {
			Fail(c, err)
			c.Abort()
			return
		}
		ctx := principal.With(c.Request.Context(), p)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

type limiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

func (l *limiter) allow(key string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	now := time.Now()
	cut := now.Add(-time.Minute)
	l.mu.Lock()
	defer l.mu.Unlock()
	list := l.hits[key]
	kept := list[:0]
	for _, t := range list {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perMin {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

func RateLimit(l *limiter, perMin int, name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := name + ":" + c.ClientIP()
		if !l.allow(key, perMin) {
			c.Header("Retry-After", "60")
			Fail(c, apperr.RateLimited("请求过于频繁，请稍后再试"))
			c.Abort()
			return
		}
		c.Next()
	}
}
