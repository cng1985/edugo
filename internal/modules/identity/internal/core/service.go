package core

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/modules/identity/api"
	"github.com/cng1985/edugo/internal/modules/identity/internal/store"
	"github.com/cng1985/edugo/internal/platform/audit"
	"github.com/cng1985/edugo/internal/platform/authn"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/pagination"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service struct {
	repo  *store.Repo
	cfg   *config.Config
	audit *audit.Logger
}

func New(repo *store.Repo, cfg *config.Config, auditLogger *audit.Logger) *Service {
	return &Service{repo: repo, cfg: cfg, audit: auditLogger}
}

func (s *Service) Login(ctx context.Context, req api.LoginRequest) (api.TokenPair, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return api.TokenPair{}, apperr.Invalid("请输入账号和密码")
	}
	user, err := s.repo.FindUserByUsername(username)
	if err != nil {
		_ = s.audit.Record(ctx, nil, audit.Entry{Action: "auth.login_failed", TargetType: "user", After: username})
		return api.TokenPair{}, apperr.Unauthorized("账号或密码错误")
	}
	if user.Status != "active" {
		_ = s.audit.Record(ctx, nil, audit.Entry{ActorID: user.ID, Action: "auth.login_denied", TargetType: "user", TargetID: user.ID, Reason: user.Status})
		return api.TokenPair{}, apperr.New(401, "ACCOUNT_DISABLED", "账号已停用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		_ = s.audit.Record(ctx, nil, audit.Entry{ActorID: user.ID, Action: "auth.login_failed", TargetType: "user", TargetID: user.ID})
		return api.TokenPair{}, apperr.Unauthorized("账号或密码错误")
	}
	return s.issue(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, req api.RefreshRequest) (api.TokenPair, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return api.TokenPair{}, apperr.Invalid("缺少刷新令牌")
	}
	hash := authn.Hash(req.RefreshToken)
	session, err := s.repo.FindSessionByCurrent(hash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if old, e2 := s.repo.FindSessionByPrevious(hash); e2 == nil {
			_ = s.repo.RevokeSession(old.ID)
			_ = s.audit.Record(ctx, nil, audit.Entry{ActorID: old.UserID, Action: "auth.refresh_reuse", TargetType: "session", TargetID: old.ID})
			return api.TokenPair{}, apperr.Unauthorized("刷新令牌已失效")
		}
		return api.TokenPair{}, apperr.Unauthorized("刷新令牌无效")
	}
	if err != nil {
		return api.TokenPair{}, err
	}
	if session.Status != "active" || time.Now().UTC().After(session.ExpiresAt) {
		return api.TokenPair{}, apperr.Unauthorized("刷新令牌已失效")
	}
	user, err := s.repo.FindUser(session.UserID)
	if err != nil {
		return api.TokenPair{}, err
	}
	if user.Status != "active" {
		_ = s.repo.RevokeSession(session.ID)
		return api.TokenPair{}, apperr.New(401, "ACCOUNT_DISABLED", "账号已停用")
	}
	plain, newHash, err := authn.NewRefresh()
	if err != nil {
		return api.TokenPair{}, apperr.Internal("签发令牌失败")
	}
	now := time.Now().UTC()
	session.PreviousHash = session.CurrentHash
	session.CurrentHash = newHash
	session.RotatedAt = &now
	session.ExpiresAt = now.Add(s.cfg.Auth.RefreshExpire)
	if err := s.repo.SaveSession(session); err != nil {
		return api.TokenPair{}, err
	}
	access, err := authn.SignAccess(s.secret(), user.ID, session.ID, "edugo", s.cfg.Auth.JWTExpire)
	if err != nil {
		return api.TokenPair{}, apperr.Internal("签发令牌失败")
	}
	view, err := s.toUser(user)
	if err != nil {
		return api.TokenPair{}, err
	}
	return api.TokenPair{AccessToken: access, RefreshToken: plain, ExpiresAt: now.Add(s.cfg.Auth.JWTExpire), User: view}, nil
}

func (s *Service) Logout(ctx context.Context) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	if err := s.repo.RevokeSession(p.SessionID); err != nil {
		return err
	}
	return s.audit.Record(ctx, nil, audit.Entry{ActorID: p.UserID, Action: "auth.logout", TargetType: "session", TargetID: p.SessionID})
}

func (s *Service) Profile(ctx context.Context) (api.User, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.User{}, err
	}
	user, err := s.repo.FindUser(p.UserID)
	if err != nil {
		return api.User{}, err
	}
	return s.toUser(user)
}

func (s *Service) UpdateProfile(ctx context.Context, req api.UpdateProfileRequest) (api.User, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.User{}, err
	}
	user, err := s.repo.FindUser(p.UserID)
	if err != nil {
		return api.User{}, err
	}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" {
			return api.User{}, apperr.Invalid("显示名不能为空")
		}
		user.DisplayName = name
	}
	if req.Timezone != nil {
		if _, err := time.LoadLocation(*req.Timezone); err != nil {
			return api.User{}, apperr.Invalid("时区无法识别")
		}
		user.Timezone = *req.Timezone
	}
	user.Version++
	if err := s.repo.SaveUser(user); err != nil {
		return api.User{}, err
	}
	return s.toUser(user)
}

func (s *Service) CreateUser(ctx context.Context, req api.CreateUserRequest) (api.User, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.User{}, err
	}
	if !p.Can("user.manage") {
		return api.User{}, apperr.Forbidden("无权创建用户")
	}
	return s.createUser(req, p.OrgID)
}

func (s *Service) createUser(req api.CreateUserRequest, fallbackOrg string) (api.User, error) {
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(req.Password) < 8 {
		return api.User{}, apperr.Invalid("用户名至少 3 位且密码至少 8 位")
	}
	if req.Role != api.RoleAdmin && req.Role != api.RoleTeacher && req.Role != api.RoleStudent {
		return api.User{}, apperr.Invalid("角色非法")
	}
	tz := req.Timezone
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return api.User{}, apperr.Invalid("时区无法识别")
	}
	orgID := req.OrgID
	if orgID == "" {
		orgID = fallbackOrg
	}
	if orgID == "" {
		orgID = "default"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.Security.BcryptCost)
	if err != nil {
		return api.User{}, apperr.Internal("密码处理失败")
	}
	role, err := s.repo.RoleByCode(req.Role)
	if err != nil {
		return api.User{}, err
	}
	display := strings.TrimSpace(req.DisplayName)
	if display == "" {
		display = username
	}
	user := store.User{
		ID: xid.New(), Username: username, PasswordHash: string(hash), DisplayName: display,
		Status: "active", Timezone: tz, OrgID: orgID, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateUser(user, role.ID); err != nil {
		return api.User{}, err
	}
	return s.toUser(user)
}

func (s *Service) UpdateUser(ctx context.Context, req api.UpdateUserRequest) (api.User, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.User{}, err
	}
	if !p.Can("user.manage") {
		return api.User{}, apperr.Forbidden("无权修改用户")
	}
	user, err := s.repo.FindUser(req.UserID)
	if err != nil {
		return api.User{}, err
	}
	if s.cfg.Organization.Enabled && user.OrgID != p.OrgID && !p.IsAdmin() {
		return api.User{}, apperr.NotFound("用户不存在")
	}
	before := user.DisplayName
	if req.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.OrgID != nil && *req.OrgID != "" {
		user.OrgID = *req.OrgID
	}
	user.Version++
	if err := s.repo.SaveUser(user); err != nil {
		return api.User{}, err
	}
	if req.Role != nil {
		role, err := s.repo.RoleByCode(*req.Role)
		if err != nil {
			return api.User{}, err
		}
		if err := s.repo.ReplaceRole(user.ID, role.ID); err != nil {
			return api.User{}, err
		}
		_ = s.audit.Record(ctx, nil, audit.Entry{ActorID: p.UserID, Action: "user.role_change", TargetType: "user", TargetID: user.ID, Before: before, After: *req.Role})
	}
	return s.toUser(user)
}

func (s *Service) DisableUser(ctx context.Context, req api.UserIDRequest) error {
	return s.setStatus(ctx, req.UserID, "disabled")
}

func (s *Service) EnableUser(ctx context.Context, req api.UserIDRequest) error {
	return s.setStatus(ctx, req.UserID, "active")
}

func (s *Service) setStatus(ctx context.Context, userID, status string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	if !p.Can("user.manage") {
		return apperr.Forbidden("无权变更账号状态")
	}
	if userID == p.UserID {
		return apperr.Invalid("不能停用当前登录账号")
	}
	user, err := s.repo.FindUser(userID)
	if err != nil {
		return err
	}
	before := user.Status
	user.Status = status
	user.Version++
	if err := s.repo.SaveUser(user); err != nil {
		return err
	}
	return s.audit.Record(ctx, nil, audit.Entry{ActorID: p.UserID, Action: "user.status", TargetType: "user", TargetID: user.ID, Before: before, After: status})
}

func (s *Service) DetailUser(ctx context.Context, req api.UserIDRequest) (api.User, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.User{}, err
	}
	if req.UserID != p.UserID && !p.Can("user.manage") {
		return api.User{}, apperr.NotFound("用户不存在")
	}
	user, err := s.repo.FindUser(req.UserID)
	if err != nil {
		return api.User{}, err
	}
	if s.cfg.Organization.Enabled && user.OrgID != p.OrgID && req.UserID != p.UserID {
		return api.User{}, apperr.NotFound("用户不存在")
	}
	return s.toUser(user)
}

func (s *Service) ListUsers(ctx context.Context, req api.ListUserRequest) ([]api.User, int64, int, int, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if !p.Can("user.manage") {
		return nil, 0, 0, 0, apperr.Forbidden("无权查看用户列表")
	}
	page, size, err := pagination.Normalize(req.Page, req.Size, s.cfg.Query.DefaultPageSize, s.cfg.Query.MaxPageSize)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	org := ""
	if s.cfg.Organization.Enabled {
		org = p.OrgID
	}
	rows, total, err := s.repo.ListUsers(org, req.Status, req.Role, pagination.Offset(page, size), size)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	out := make([]api.User, 0, len(rows))
	for _, row := range rows {
		view, err := s.toUser(row)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		out = append(out, view)
	}
	return out, total, page, size, nil
}

func (s *Service) ListRoles(ctx context.Context) ([]api.Role, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	if !p.Can("user.manage") {
		return nil, apperr.Forbidden("无权查看角色")
	}
	rows, err := s.repo.ListRoles()
	if err != nil {
		return nil, err
	}
	out := make([]api.Role, 0, len(rows))
	for _, row := range rows {
		perms, err := s.repo.PermissionsOfRole(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, api.Role{Code: row.Code, Name: row.Name, Permissions: perms})
	}
	return out, nil
}

func (s *Service) UpdateRole(ctx context.Context, req api.UpdateRoleRequest) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	if !p.Can("user.manage") {
		return apperr.Forbidden("无权修改角色")
	}
	role, err := s.repo.RoleByCode(req.Code)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(req.Permissions))
	for _, code := range req.Permissions {
		perm, err := s.repo.PermissionByCode(code)
		if err != nil {
			return err
		}
		ids = append(ids, perm.ID)
	}
	if err := s.repo.ReplaceRolePermissions(role.ID, ids); err != nil {
		return err
	}
	return s.audit.Record(ctx, nil, audit.Entry{ActorID: p.UserID, Action: "role.permissions", TargetType: "role", TargetID: role.ID, After: strings.Join(req.Permissions, ",")})
}

func (s *Service) CreateAgent(ctx context.Context, req api.CreateAgentRequest) (api.AgentCredential, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.AgentCredential{}, err
	}
	if !p.IsAdmin() {
		return api.AgentCredential{}, apperr.Forbidden("无权创建 Agent 凭据")
	}
	user, err := s.repo.FindUser(req.DelegatedUserID)
	if err != nil {
		return api.AgentCredential{}, err
	}
	plain, hash, err := authn.NewRefresh()
	if err != nil {
		return api.AgentCredential{}, apperr.Internal("生成凭据失败")
	}
	row := store.AgentCredential{
		ID: xid.New(), Name: req.Name, SecretHash: hash, DelegatedUserID: user.ID, CreatedBy: p.UserID, Status: "active", CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateAgent(row); err != nil {
		return api.AgentCredential{}, err
	}
	if err := s.audit.Record(ctx, nil, audit.Entry{ActorID: p.UserID, Action: "agent.create", TargetType: "agent", TargetID: row.ID, After: user.ID}); err != nil {
		return api.AgentCredential{}, err
	}
	return api.AgentCredential{ID: row.ID, Name: row.Name, Token: row.ID + "." + plain, Delegated: user.ID, Status: row.Status}, nil
}

func (s *Service) ResolveSession(ctx context.Context, userID, sessionID string) (principal.Principal, error) {
	session, err := s.repo.FindSession(sessionID)
	if err != nil {
		return principal.Principal{}, apperr.Unauthorized("会话已失效")
	}
	if session.UserID != userID || session.Status != "active" || time.Now().UTC().After(session.ExpiresAt) {
		return principal.Principal{}, apperr.Unauthorized("会话已失效")
	}
	user, err := s.repo.FindUser(userID)
	if err != nil {
		return principal.Principal{}, apperr.Unauthorized("会话已失效")
	}
	if user.Status != "active" {
		return principal.Principal{}, apperr.New(401, "ACCOUNT_DISABLED", "账号已停用")
	}
	return s.principalOf(user, session.ID, false, "", "")
}

func (s *Service) ResolveAgent(ctx context.Context, token string) (principal.Principal, error) {
	id, secret, ok := strings.Cut(token, ".")
	if !ok || id == "" || secret == "" {
		return principal.Principal{}, apperr.Unauthorized("Agent 凭据无效")
	}
	row, err := s.repo.FindAgent(id)
	if err != nil {
		return principal.Principal{}, err
	}
	if row.Status != "active" || row.SecretHash != authn.Hash(secret) {
		return principal.Principal{}, apperr.Unauthorized("Agent 凭据无效")
	}
	user, err := s.repo.FindUser(row.DelegatedUserID)
	if err != nil || user.Status != "active" {
		return principal.Principal{}, apperr.Unauthorized("委托用户不可用")
	}
	return s.principalOf(user, "", true, row.ID, row.CreatedBy)
}

func (s *Service) principalOf(user store.User, sessionID string, agent bool, agentID, delegatedBy string) (principal.Principal, error) {
	roles, perms, err := s.repo.RolesOf(user.ID)
	if err != nil {
		return principal.Principal{}, err
	}
	org := user.OrgID
	if !s.cfg.Organization.Enabled {
		org = "default"
	}
	return principal.Principal{
		UserID: user.ID, SessionID: sessionID, Username: user.Username, DisplayName: user.DisplayName,
		Roles: roles, Permissions: perms, Timezone: user.Timezone, OrgID: org,
		Agent: agent, AgentID: agentID, DelegatedBy: delegatedBy,
	}, nil
}

func (s *Service) issue(ctx context.Context, user store.User) (api.TokenPair, error) {
	plain, hash, err := authn.NewRefresh()
	if err != nil {
		return api.TokenPair{}, apperr.Internal("签发令牌失败")
	}
	now := time.Now().UTC()
	session := store.Session{
		ID: xid.New(), UserID: user.ID, CurrentHash: hash, Status: "active",
		ExpiresAt: now.Add(s.cfg.Auth.RefreshExpire), CreatedAt: now,
	}
	if err := s.repo.CreateSession(session); err != nil {
		return api.TokenPair{}, err
	}
	access, err := authn.SignAccess(s.secret(), user.ID, session.ID, "edugo", s.cfg.Auth.JWTExpire)
	if err != nil {
		return api.TokenPair{}, apperr.Internal("签发令牌失败")
	}
	view, err := s.toUser(user)
	if err != nil {
		return api.TokenPair{}, err
	}
	_ = s.audit.Record(ctx, nil, audit.Entry{ActorID: user.ID, Action: "auth.login", TargetType: "session", TargetID: session.ID})
	return api.TokenPair{AccessToken: access, RefreshToken: plain, ExpiresAt: now.Add(s.cfg.Auth.JWTExpire), User: view}, nil
}

func (s *Service) toUser(user store.User) (api.User, error) {
	roles, _, err := s.repo.RolesOf(user.ID)
	if err != nil {
		return api.User{}, err
	}
	return api.User{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Status: user.Status,
		Timezone: user.Timezone, OrgID: user.OrgID, Roles: roles, CreatedAt: user.CreatedAt,
	}, nil
}

func (s *Service) secret() string {
	if s.cfg.Auth.JWTSecret != "" {
		return s.cfg.Auth.JWTSecret
	}
	return ephemeralSecret
}

var ephemeralSecret = xid.New() + xid.New()

func (s *Service) Seed(ctx context.Context) error {
	perms := []store.Permission{}
	codes := []string{"user.manage", "course.manage", "question.manage", "exam.manage", "exam.review", "exam.publishResult", "certificate.issue", "certificate.revoke", "report.read", "ai.configure", "export", "chat.audit", "org.manage"}
	for _, code := range codes {
		perms = append(perms, store.Permission{ID: "perm-" + code, Code: code})
	}
	roles := []store.Role{
		{ID: "role-admin", Code: api.RoleAdmin, Name: "管理员"},
		{ID: "role-teacher", Code: api.RoleTeacher, Name: "教师"},
		{ID: "role-student", Code: api.RoleStudent, Name: "学生"},
	}
	links := map[string][]string{
		api.RoleAdmin:   codes,
		api.RoleTeacher: {"course.manage", "question.manage", "exam.manage", "exam.review", "exam.publishResult", "report.read"},
	}
	if err := s.repo.EnsureRBAC(roles, perms, links); err != nil {
		return err
	}
	username, password, explicit := s.adminCredentials()
	if password == "" {
		return nil
	}
	user, err := s.repo.FindUserByUsername(username)
	if err != nil {
		if !apperr.IsCode(err, "UNAUTHENTICATED") {
			return err
		}
		if _, err := s.createUser(api.CreateUserRequest{
			Username: username, Password: password, DisplayName: "管理员", Role: api.RoleAdmin, Timezone: "Asia/Shanghai", OrgID: "default",
		}, "default"); err != nil && !apperr.IsCode(err, "USERNAME_TAKEN") {
			return err
		}
		if !explicit {
			slog.Info("已创建开发默认管理员", "username", username)
		}
		return nil
	}
	if !explicit {
		return nil
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.Security.BcryptCost)
	if err != nil {
		return apperr.Internal("密码处理失败")
	}
	user.PasswordHash = string(hash)
	user.Version++
	user.UpdatedAt = time.Now().UTC()
	return s.repo.SaveUser(user)
}

const debugAdminPassword = "admin123"

func (s *Service) adminCredentials() (username, password string, explicit bool) {
	username = strings.TrimSpace(s.cfg.Admin.Username)
	if username == "" {
		username = "admin"
	}
	password = strings.TrimSpace(s.cfg.Admin.Password)
	explicit = password != ""
	if !explicit && strings.EqualFold(s.cfg.Server.Mode, "debug") {
		password = debugAdminPassword
	}
	return username, password, explicit
}

func (s *Service) BootstrapHint() *api.BootstrapHint {
	username, password, explicit := s.adminCredentials()
	if explicit || password == "" {
		return nil
	}
	return &api.BootstrapHint{Username: username, Password: password}
}

func (s *Service) FindByUsername(username string) (api.User, error) {
	user, err := s.repo.FindUserByUsername(username)
	if err != nil {
		if apperr.As(err).ErrorCode == "UNAUTHENTICATED" {
			return api.User{}, apperr.NotFound("用户不存在")
		}
		return api.User{}, err
	}
	return s.toUser(user)
}

func (s *Service) BootstrapUser(req api.CreateUserRequest) (api.User, error) {
	return s.createUser(req, "default")
}
