package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Record struct {
	ID           string `gorm:"primaryKey;size:36"`
	Scope        string `gorm:"size:64;not null;uniqueIndex:uk_idem"`
	ActorID      string `gorm:"size:36;not null;uniqueIndex:uk_idem"`
	Endpoint     string `gorm:"size:128;not null;uniqueIndex:uk_idem"`
	IdemKey      string `gorm:"size:128;not null;uniqueIndex:uk_idem"`
	RequestHash  string `gorm:"size:64;not null"`
	StatusCode   int
	ResponseBody string `gorm:"type:text"`
	CreatedAt    time.Time
}

func (Record) TableName() string { return "idempotency_records" }

func Models() []any { return []any{&Record{}} }

func Hash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type Store struct{ db *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Lookup(scope, actor, endpoint, key, hash string) (found bool, code int, body string, err error) {
	if key == "" {
		return false, 0, "", nil
	}
	var row Record
	err = s.db.Where("scope = ? AND actor_id = ? AND endpoint = ? AND idem_key = ?", scope, actor, endpoint, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, 0, "", nil
	}
	if err != nil {
		return false, 0, "", err
	}
	if row.RequestHash != hash {
		return false, 0, "", apperr.Conflict("IDEMPOTENCY_CONFLICT", "相同幂等键的请求内容不一致")
	}
	return true, row.StatusCode, row.ResponseBody, nil
}

func (s *Store) Save(scope, actor, endpoint, key, hash string, code int, body string) error {
	if key == "" {
		return nil
	}
	row := Record{
		ID: xid.New(), Scope: scope, ActorID: actor, Endpoint: endpoint, IdemKey: key,
		RequestHash: hash, StatusCode: code, ResponseBody: body, CreatedAt: time.Now().UTC(),
	}
	err := s.db.Create(&row).Error
	if err != nil {
		var existing Record
		if e := s.db.Where("scope = ? AND actor_id = ? AND endpoint = ? AND idem_key = ?", scope, actor, endpoint, key).First(&existing).Error; e == nil {
			if existing.RequestHash != hash {
				return apperr.Conflict("IDEMPOTENCY_CONFLICT", "相同幂等键的请求内容不一致")
			}
			return nil
		}
	}
	return err
}
