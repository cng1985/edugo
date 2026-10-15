package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Object struct {
	ID          string `gorm:"primaryKey;size:36"`
	OwnerID     string `gorm:"size:36;index;not null"`
	OrgID       string `gorm:"size:36;index"`
	Purpose     string `gorm:"size:64;index"`
	Filename    string `gorm:"size:255"`
	ContentType string `gorm:"size:128"`
	Size        int64
	SHA256      string `gorm:"size:64"`
	RelPath     string `gorm:"size:255;not null"`
	CreatedAt   time.Time
}

func (Object) TableName() string { return "storage_objects" }

func Models() []any { return []any{&Object{}} }

type Store struct {
	db  *gorm.DB
	cfg *config.Config
}

func New(db *gorm.DB, cfg *config.Config) *Store { return &Store{db: db, cfg: cfg} }

func (s *Store) Save(ownerID, orgID, purpose, filename, contentType string, r io.Reader) (Object, error) {
	if s.cfg.Storage.Driver != "local" {
		return Object{}, apperr.Unavailable("STORAGE_DRIVER", "不支持的存储驱动")
	}
	if err := os.MkdirAll(s.cfg.Storage.Root, 0o750); err != nil {
		return Object{}, apperr.Internal("创建上传目录失败")
	}
	id := xid.New()
	safeName := filepath.Base(filename)
	rel := filepath.Join(id[:2], id+filepath.Ext(safeName))
	full := filepath.Join(s.cfg.Storage.Root, rel)
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(s.cfg.Storage.Root)) {
		return Object{}, apperr.Invalid("文件路径非法")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return Object{}, apperr.Internal("创建上传目录失败")
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return Object{}, apperr.Internal("写入文件失败")
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, s.cfg.Storage.MaxBytes+1))
	if err != nil {
		return Object{}, apperr.Internal("写入文件失败")
	}
	if n > s.cfg.Storage.MaxBytes {
		_ = os.Remove(full)
		return Object{}, apperr.TooLarge("文件超过大小限制")
	}
	obj := Object{
		ID: id, OwnerID: ownerID, OrgID: orgID, Purpose: purpose, Filename: safeName,
		ContentType: contentType, Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), RelPath: rel, CreatedAt: time.Now().UTC(),
	}
	if err := s.db.Create(&obj).Error; err != nil {
		return Object{}, err
	}
	return obj, nil
}

func (s *Store) Get(id string) (Object, string, error) {
	var obj Object
	if err := s.db.Where("id = ?", id).First(&obj).Error; err != nil {
		return Object{}, "", apperr.NotFound("文件不存在")
	}
	full := filepath.Join(s.cfg.Storage.Root, obj.RelPath)
	cleanRoot := filepath.Clean(s.cfg.Storage.Root)
	cleanFull := filepath.Clean(full)
	if !strings.HasPrefix(cleanFull, cleanRoot) {
		return Object{}, "", apperr.Invalid("文件路径非法")
	}
	if _, err := os.Stat(cleanFull); err != nil {
		return Object{}, "", apperr.NotFound("文件内容不可用")
	}
	return obj, cleanFull, nil
}

func (s *Store) Exists(id string) bool {
	_, path, err := s.Get(id)
	return err == nil && path != ""
}
