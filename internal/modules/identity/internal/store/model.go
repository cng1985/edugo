package store

import "time"

type User struct {
	ID           string `gorm:"primaryKey;size:36"`
	Username     string `gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	DisplayName  string `gorm:"size:128;not null"`
	Status       string `gorm:"size:32;index;not null"`
	Timezone     string `gorm:"size:64;not null"`
	OrgID        string `gorm:"size:36;index;not null"`
	Version      int64  `gorm:"not null;default:1"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (User) TableName() string { return "users" }

type Role struct {
	ID   string `gorm:"primaryKey;size:36"`
	Code string `gorm:"size:32;uniqueIndex;not null"`
	Name string `gorm:"size:64;not null"`
}

func (Role) TableName() string { return "roles" }

type Permission struct {
	ID   string `gorm:"primaryKey;size:36"`
	Code string `gorm:"size:64;uniqueIndex;not null"`
}

func (Permission) TableName() string { return "permissions" }

type UserRole struct {
	ID     string `gorm:"primaryKey;size:36"`
	UserID string `gorm:"size:36;not null;uniqueIndex:uk_user_role"`
	RoleID string `gorm:"size:36;not null;uniqueIndex:uk_user_role"`
}

func (UserRole) TableName() string { return "user_roles" }

type RolePermission struct {
	ID           string `gorm:"primaryKey;size:36"`
	RoleID       string `gorm:"size:36;not null;uniqueIndex:uk_role_perm"`
	PermissionID string `gorm:"size:36;not null;uniqueIndex:uk_role_perm"`
}

func (RolePermission) TableName() string { return "role_permissions" }

type Session struct {
	ID           string `gorm:"primaryKey;size:36"`
	UserID       string `gorm:"size:36;index;not null"`
	CurrentHash  string `gorm:"size:64;uniqueIndex;not null"`
	PreviousHash string `gorm:"size:64;index"`
	Status       string `gorm:"size:32;index;not null"`
	ExpiresAt    time.Time
	CreatedAt    time.Time
	RotatedAt    *time.Time
}

func (Session) TableName() string { return "auth_sessions" }

type AgentCredential struct {
	ID              string `gorm:"primaryKey;size:36"`
	Name            string `gorm:"size:128;not null"`
	SecretHash      string `gorm:"size:64;uniqueIndex;not null"`
	DelegatedUserID string `gorm:"size:36;index;not null"`
	CreatedBy       string `gorm:"size:36;not null"`
	Status          string `gorm:"size:32;not null"`
	CreatedAt       time.Time
}

func (AgentCredential) TableName() string { return "agent_credentials" }

func Models() []any {
	return []any{&User{}, &Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &Session{}, &AgentCredential{}}
}
