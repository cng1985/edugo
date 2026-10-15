package httpapi

import (
	"github.com/cng1985/edugo/internal/modules/identity/api"
	"github.com/cng1985/edugo/internal/modules/identity/internal/core"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *core.Service }

func New(svc *core.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(public, authed *gin.RouterGroup) {
	public.POST("/auth/login", h.login)
	public.POST("/auth/refresh", h.refresh)
	authed.POST("/auth/logout", h.logout)
	authed.POST("/auth/profile", h.profile)
	authed.POST("/auth/updateProfile", h.updateProfile)
	authed.POST("/user/create", h.createUser)
	authed.POST("/user/update", h.updateUser)
	authed.POST("/user/detail", h.detailUser)
	authed.POST("/user/list", h.listUsers)
	authed.POST("/user/disable", h.disable)
	authed.POST("/user/enable", h.enable)
	authed.POST("/role/list", h.roles)
	authed.POST("/role/update", h.updateRole)
	authed.POST("/agent/credential/create", h.createAgent)
}

func (h *Handler) login(c *gin.Context) {
	var req api.LoginRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) refresh(c *gin.Context) {
	var req api.RefreshRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.Refresh(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) logout(c *gin.Context) {
	if err := h.svc.Logout(c.Request.Context()); err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, gin.H{"loggedOut": true})
}

func (h *Handler) profile(c *gin.Context) {
	out, err := h.svc.Profile(c.Request.Context())
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) updateProfile(c *gin.Context) {
	var req api.UpdateProfileRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateProfile(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) createUser(c *gin.Context) {
	var req api.CreateUserRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateUser(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) updateUser(c *gin.Context) {
	var req api.UpdateUserRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateUser(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) detailUser(c *gin.Context) {
	var req api.UserIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.DetailUser(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) listUsers(c *gin.Context) {
	var req api.ListUserRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, total, page, size, err := h.svc.ListUsers(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.Page(c, out, page, size, total)
}

func (h *Handler) disable(c *gin.Context) {
	var req api.UserIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	if err := h.svc.DisableUser(c.Request.Context(), req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, gin.H{"status": "disabled"})
}

func (h *Handler) enable(c *gin.Context) {
	var req api.UserIDRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	if err := h.svc.EnableUser(c.Request.Context(), req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, gin.H{"status": "active"})
}

func (h *Handler) roles(c *gin.Context) {
	out, err := h.svc.ListRoles(c.Request.Context())
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}

func (h *Handler) updateRole(c *gin.Context) {
	var req api.UpdateRoleRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	if err := h.svc.UpdateRole(c.Request.Context(), req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, gin.H{"updated": true})
}

func (h *Handler) createAgent(c *gin.Context) {
	var req api.CreateAgentRequest
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	out, err := h.svc.CreateAgent(c.Request.Context(), req)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, out)
}
