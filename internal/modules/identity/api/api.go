package api

import "time"

type Accounts interface {
	BootstrapUser(req CreateUserRequest) (User, error)
	FindByUsername(username string) (User, error)
}

const (
	RoleAdmin   = "admin"
	RoleTeacher = "teacher"
	RoleStudent = "student"
)

type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	User         User      `json:"user"`
}

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Status      string    `json:"status"`
	Timezone    string    `json:"timezone"`
	OrgID       string    `json:"orgId"`
	Roles       []string  `json:"roles"`
	CreatedAt   time.Time `json:"createdAt"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName"`
	Timezone    *string `json:"timezone"`
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Timezone    string `json:"timezone"`
	OrgID       string `json:"orgId"`
}

type UpdateUserRequest struct {
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Role        *string `json:"role"`
	OrgID       *string `json:"orgId"`
}

type UserIDRequest struct {
	UserID string `json:"userId"`
}

type ListUserRequest struct {
	Page   int    `json:"page"`
	Size   int    `json:"size"`
	Status string `json:"status"`
	Role   string `json:"role"`
}

type Role struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type UpdateRoleRequest struct {
	Code        string   `json:"code"`
	Permissions []string `json:"permissions"`
}

type CreateAgentRequest struct {
	Name             string `json:"name"`
	DelegatedUserID  string `json:"delegatedUserId"`
}

type AgentCredential struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token,omitempty"`
	Delegated string `json:"delegatedUserId"`
	Status    string `json:"status"`
}
