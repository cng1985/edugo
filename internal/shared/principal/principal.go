package principal

import (
	"context"

	"github.com/cng1985/edugo/internal/shared/apperr"
)

// Principal 是服务端解析后的可信身份，不来自客户端自行声明的角色或用户 ID。
type Principal struct {
	UserID      string
	SessionID   string
	Username    string
	DisplayName string
	Roles       []string
	Permissions map[string]bool
	Timezone    string
	OrgID       string
	Agent       bool
	AgentID     string
	DelegatedBy string
}

func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func (p Principal) IsAdmin() bool { return p.HasRole("admin") }

func (p Principal) Can(perm string) bool {
	if p.IsAdmin() {
		return true
	}
	return p.Permissions != nil && p.Permissions[perm]
}

func (p Principal) CanLearn() bool {
	return p.HasRole("student") || p.HasRole("teacher")
}

type ctxKey struct{}

func With(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok && p.UserID != ""
}

func Must(ctx context.Context) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, apperr.Unauthorized("未登录或会话已失效")
	}
	return p, nil
}
