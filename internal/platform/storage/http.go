package storage

import (
	"os"

	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

type Handler struct{ store *Store }

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/file/upload", h.upload)
	g.POST("/file/download", h.download)
}

func (h *Handler) upload(c *gin.Context) {
	p, err := principal.Must(c.Request.Context())
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpserver.Fail(c, apperr.Invalid("请使用 multipart 上传文件"))
		return
	}
	defer file.Close()
	obj, err := h.store.Save(p.UserID, p.OrgID, c.PostForm("purpose"), header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, gin.H{"id": obj.ID, "filename": obj.Filename, "size": obj.Size})
}

func (h *Handler) download(c *gin.Context) {
	p, err := principal.Must(c.Request.Context())
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	obj, path, err := h.store.Get(req.ID)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	if obj.OwnerID != p.UserID && !p.IsAdmin() {
		httpserver.Fail(c, apperr.NotFound("文件不存在"))
		return
	}
	c.File(path)
}

func init() { _ = os.ErrNotExist }

var HTTPModule = fx.Module("storage-http",
	fx.Provide(NewHandler),
	fx.Provide(fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`))),
)
