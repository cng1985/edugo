package database

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Ready struct{}

type Contribution struct {
	Models []any
	After  func(*gorm.DB) error
}

func Open(cfg *config.Config) (*gorm.DB, error) {
	gcfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Warn), NowFunc: func() time.Time { return time.Now().UTC() }}
	var dialector gorm.Dialector
	switch cfg.Database.Driver {
	case "sqlite":
		dsn := cfg.Database.DSN
		if err := ensureSQLiteDir(dsn); err != nil {
			return nil, err
		}
		if !strings.Contains(dsn, "_pragma=foreign_keys") {
			sep := "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
			dsn += sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
		}
		dialector = sqlite.Open(dsn)
	case "mysql":
		dialector = mysql.Open(cfg.Database.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动")
	}
	db, err := gorm.Open(dialector, gcfg)
	if err != nil {
		if cfg.Database.Driver == "sqlite" {
			return nil, fmt.Errorf("数据库连接失败（驱动 sqlite）: %w", err)
		}
		return nil, fmt.Errorf("数据库连接失败（驱动 %s）", cfg.Database.Driver)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败")
	}
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	if cfg.Database.Driver == "sqlite" {
		if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			return nil, fmt.Errorf("启用 sqlite 外键失败")
		}
	}
	return db, nil
}

type migrationRow struct {
	Version    string `gorm:"primaryKey;size:64"`
	Name       string `gorm:"size:128;not null"`
	Checksum   string `gorm:"size:128;not null"`
	StartedAt  time.Time
	FinishedAt *time.Time
	Success    bool
}

func (migrationRow) TableName() string { return "schema_migrations" }

func Migrate(db *gorm.DB, parts []Contribution) (Ready, error) {
	if err := db.AutoMigrate(&migrationRow{}); err != nil {
		return Ready{}, fmt.Errorf("迁移记录表创建失败")
	}
	const version = "0001_baseline"
	var existing migrationRow
	err := db.Where("version = ?", version).First(&existing).Error
	if err == nil && existing.Success {
		return Ready{}, nil
	}
	started := time.Now().UTC()
	row := migrationRow{Version: version, Name: "baseline", Checksum: "edugo-schema-v1", StartedAt: started, Success: false}
	if err := db.Save(&row).Error; err != nil {
		return Ready{}, fmt.Errorf("写入迁移记录失败")
	}
	models := make([]any, 0)
	for _, part := range parts {
		models = append(models, part.Models...)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			return Ready{}, fmt.Errorf("版本化迁移失败")
		}
	}
	for _, part := range parts {
		if part.After != nil {
			if err := part.After(db); err != nil {
				return Ready{}, fmt.Errorf("版本化索引迁移失败")
			}
		}
	}
	finished := time.Now().UTC()
	row.FinishedAt = &finished
	row.Success = true
	if err := db.Save(&row).Error; err != nil {
		return Ready{}, fmt.Errorf("更新迁移记录失败")
	}
	return Ready{}, nil
}

func UpdateVersion(tx *gorm.DB, model any, id string, expected int64, updates map[string]any) error {
	updates["version"] = expected + 1
	updates["updated_at"] = time.Now().UTC()
	res := tx.Model(model).Where("id = ? AND version = ?", id, expected).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}

var ErrVersionConflict = errors.New("version conflict")

// ensureSQLiteDir 创建文件库的上级目录。SQLite 只会创建库文件本身，默认 DSN data/edugo.db
// 在 data 目录不存在时会返回 SQLITE_CANTOPEN。内存库不落地，不创建目录。
func ensureSQLiteDir(dsn string) error {
	dir, ok := sqliteParentDir(dsn)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("创建 sqlite 数据目录失败: %w", err)
	}
	return nil
}

func sqliteParentDir(dsn string) (string, bool) {
	path, ok := sqliteFilePath(dsn)
	if !ok {
		return "", false
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return "", false
	}
	return dir, true
}

func sqliteFilePath(dsn string) (string, bool) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || dsn == ":memory:" {
		return "", false
	}
	name := dsn
	query := ""
	if i := strings.IndexRune(dsn, '?'); i >= 0 {
		name = dsn[:i]
		query = dsn[i+1:]
	}
	if q, err := url.ParseQuery(query); err == nil && strings.EqualFold(q.Get("mode"), "memory") {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(name), "file:") {
		rest := name[len("file:"):]
		if rest == ":memory:" || strings.HasPrefix(rest, ":memory:") {
			return "", false
		}
		if strings.HasPrefix(rest, "/") {
			u, err := url.Parse(name)
			if err != nil {
				return "", false
			}
			p := u.Path
			if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
				p = p[1:]
			}
			return p, p != "" && p != ":memory:"
		}
		return rest, rest != ""
	}
	return name, true
}
