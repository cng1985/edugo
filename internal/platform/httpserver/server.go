package httpserver

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/web"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

type Registrar interface {
	Register(public, authed *gin.RouterGroup)
}

type Server struct {
	Engine *gin.Engine
	HTTP   *http.Server
	cfg    *config.Config
	log    *slog.Logger
}

type Params struct {
	fx.In
	Cfg      *config.Config
	Log      *slog.Logger
	Sessions SessionResolver
	Agents   AgentResolver
	Routes   []Registrar `group:"routes"`
}

func New(p Params) (*Server, error) {
	if p.Cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	assets, err := web.Assets()
	if err != nil {
		return nil, err
	}
	r := gin.New()
	r.Use(Trace(), Recover(p.Log), AccessLog(p.Log), LimitBody(int64(p.Cfg.Security.MaxJSONBodyBytes)))
	lim := newLimiter()
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })
	public := r.Group("/api/v1")
	public.Use(func(c *gin.Context) {
		switch c.Request.URL.Path {
		case "/api/v1/auth/login":
			RateLimit(lim, p.Cfg.Security.LoginPerMinute, "login")(c)
		case "/api/v1/auth/refresh":
			RateLimit(lim, p.Cfg.Security.RefreshPerMinute, "refresh")(c)
		}
	})
	authed := r.Group("/api/v1")
	authed.Use(Authenticate(p.Cfg, p.Sessions, p.Agents))
	for _, route := range p.Routes {
		route.Register(public, authed)
	}
	mountSPA(r, assets)
	return &Server{
		Engine: r,
		cfg:    p.Cfg,
		log:    p.Log,
		HTTP: &http.Server{
			Addr:              ":" + itoa(p.Cfg.Server.Port),
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}, nil
}

func (s *Server) Start() error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listen", "addr", s.HTTP.Addr)
		if err := s.HTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func (s *Server) Stop(ctx context.Context) error {
	return s.HTTP.Shutdown(ctx)
}

func mountSPA(r *gin.Engine, assets fs.FS) {
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			Fail(c, apperr.NotFound("接口不存在"))
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		clean := strings.TrimPrefix(path, "/")
		if clean == "" || strings.HasSuffix(clean, "/") {
			clean = "index.html"
		}
		if strings.Contains(clean, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		if serveAsset(c, assets, clean) {
			return
		}
		if strings.Contains(clean, ".") {
			c.Status(http.StatusNotFound)
			return
		}
		if !serveAsset(c, assets, "index.html") {
			c.Status(http.StatusNotFound)
		}
	})
}

func serveAsset(c *gin.Context, assets fs.FS, name string) bool {
	file, err := assets.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), seeker)
	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
