package store

import (
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

func newID() string { return xid.New() }

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) FindUserByUsername(username string) (User, error) {
	var user User
	err := r.db.Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, apperr.Unauthorized("账号或密码错误")
	}
	return user, err
}

func (r *Repo) FindUser(id string) (User, error) {
	var user User
	err := r.db.Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, apperr.NotFound("用户不存在")
	}
	return user, err
}

func (r *Repo) CreateUser(user User, roleID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return apperr.Conflict("USERNAME_TAKEN", "用户名已存在")
		}
		return tx.Create(&UserRole{ID: newID(), UserID: user.ID, RoleID: roleID}).Error
	})
}

func (r *Repo) SaveUser(user User) error { return r.db.Save(&user).Error }

func (r *Repo) ListUsers(orgFilter string, status, role string, offset, limit int) ([]User, int64, error) {
	q := r.db.Model(&User{})
	if orgFilter != "" {
		q = q.Where("org_id = ?", orgFilter)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if role != "" {
		q = q.Joins("join user_roles ur on ur.user_id = users.id").Joins("join roles ro on ro.id = ur.role_id").Where("ro.code = ?", role)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []User
	err := q.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

func (r *Repo) RolesOf(userID string) ([]string, map[string]bool, error) {
	var codes []string
	err := r.db.Table("roles").Select("roles.code").
		Joins("join user_roles on user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).Scan(&codes).Error
	if err != nil {
		return nil, nil, err
	}
	perms := map[string]bool{}
	var pcodes []string
	err = r.db.Table("permissions").Select("permissions.code").
		Joins("join role_permissions on role_permissions.permission_id = permissions.id").
		Joins("join user_roles on user_roles.role_id = role_permissions.role_id").
		Where("user_roles.user_id = ?", userID).Scan(&pcodes).Error
	if err != nil {
		return nil, nil, err
	}
	for _, code := range pcodes {
		perms[code] = true
	}
	return codes, perms, nil
}

func (r *Repo) RoleByCode(code string) (Role, error) {
	var role Role
	err := r.db.Where("code = ?", code).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Role{}, apperr.Invalid("角色不存在")
	}
	return role, err
}

func (r *Repo) ReplaceRole(userID, roleID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&UserRole{}).Error; err != nil {
			return err
		}
		return tx.Create(&UserRole{ID: newID(), UserID: userID, RoleID: roleID}).Error
	})
}

func (r *Repo) CreateSession(s Session) error { return r.db.Create(&s).Error }

func (r *Repo) FindSession(id string) (Session, error) {
	var s Session
	err := r.db.Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, apperr.Unauthorized("会话不存在")
	}
	return s, err
}

func (r *Repo) FindSessionByCurrent(hash string) (Session, error) {
	var s Session
	err := r.db.Where("current_hash = ?", hash).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, gorm.ErrRecordNotFound
	}
	return s, err
}

func (r *Repo) FindSessionByPrevious(hash string) (Session, error) {
	var s Session
	err := r.db.Where("previous_hash = ? AND previous_hash <> ''", hash).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, gorm.ErrRecordNotFound
	}
	return s, err
}

func (r *Repo) SaveSession(s Session) error { return r.db.Save(&s).Error }

func (r *Repo) RevokeSession(id string) error {
	return r.db.Model(&Session{}).Where("id = ?", id).Update("status", "revoked").Error
}

func (r *Repo) ListRoles() ([]Role, error) {
	var rows []Role
	err := r.db.Order("code asc").Find(&rows).Error
	return rows, err
}

func (r *Repo) PermissionsOfRole(roleID string) ([]string, error) {
	var codes []string
	err := r.db.Table("permissions").Select("permissions.code").
		Joins("join role_permissions on role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).Scan(&codes).Error
	return codes, err
}

func (r *Repo) ReplaceRolePermissions(roleID string, permIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&RolePermission{}).Error; err != nil {
			return err
		}
		for _, id := range permIDs {
			if err := tx.Create(&RolePermission{ID: newID(), RoleID: roleID, PermissionID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) PermissionByCode(code string) (Permission, error) {
	var p Permission
	err := r.db.Where("code = ?", code).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Permission{}, apperr.Invalid("权限不存在")
	}
	return p, err
}

func (r *Repo) EnsureRBAC(roles []Role, perms []Permission, links map[string][]string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, role := range roles {
			var count int64
			tx.Model(&Role{}).Where("code = ?", role.Code).Count(&count)
			if count == 0 {
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			}
		}
		for _, perm := range perms {
			var count int64
			tx.Model(&Permission{}).Where("code = ?", perm.Code).Count(&count)
			if count == 0 {
				if err := tx.Create(&perm).Error; err != nil {
					return err
				}
			}
		}
		for roleCode, permCodes := range links {
			var role Role
			if err := tx.Where("code = ?", roleCode).First(&role).Error; err != nil {
				return err
			}
			for _, code := range permCodes {
				var perm Permission
				if err := tx.Where("code = ?", code).First(&perm).Error; err != nil {
					return err
				}
				var count int64
				tx.Model(&RolePermission{}).Where("role_id = ? AND permission_id = ?", role.ID, perm.ID).Count(&count)
				if count == 0 {
					if err := tx.Create(&RolePermission{ID: newID(), RoleID: role.ID, PermissionID: perm.ID}).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func (r *Repo) CountUsers() (int64, error) {
	var n int64
	err := r.db.Model(&User{}).Count(&n).Error
	return n, err
}

func (r *Repo) CreateAgent(row AgentCredential) error { return r.db.Create(&row).Error }

func (r *Repo) FindAgent(id string) (AgentCredential, error) {
	var row AgentCredential
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AgentCredential{}, apperr.Unauthorized("Agent 凭据无效")
	}
	return row, err
}

func (r *Repo) TouchNow() time.Time { return time.Now().UTC() }
