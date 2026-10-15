package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cng1985/edugo/internal/platform/config"
)

func TestSQLiteParentDir(t *testing.T) {
	cases := []struct {
		dsn  string
		want string
		ok   bool
	}{
		{dsn: "data/edugo.db", want: filepath.Dir("data/edugo.db"), ok: true},
		{dsn: "data/edugo.db?_pragma=foreign_keys(1)", want: filepath.Dir("data/edugo.db"), ok: true},
		{dsn: "file:data/edugo.db?_pragma=foreign_keys(1)", want: filepath.Dir("data/edugo.db"), ok: true},
		{dsn: "file:/tmp/edugo/app.db", want: filepath.Dir("/tmp/edugo/app.db"), ok: true},
		{dsn: "file:///tmp/edugo/app.db", want: filepath.Dir("/tmp/edugo/app.db"), ok: true},
		{dsn: "file:///C:/data/edugo.db", want: filepath.Dir("C:/data/edugo.db"), ok: true},
		{dsn: "edugo.db", ok: false},
		{dsn: ":memory:", ok: false},
		{dsn: "file::memory:?cache=shared", ok: false},
		{dsn: "file:memdb?mode=memory&cache=shared", ok: false},
	}
	for _, tc := range cases {
		got, ok := sqliteParentDir(tc.dsn)
		if ok != tc.ok || got != tc.want {
			t.Errorf("sqliteParentDir(%q) = (%q, %v), want (%q, %v)", tc.dsn, got, ok, tc.want, tc.ok)
		}
	}
}

func TestOpenCreatesMissingSQLiteDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := config.Defaults()
	cfg.Database.DSN = "data/edugo.db"
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := os.Stat(filepath.Join(dir, "data", "edugo.db")); err != nil {
		t.Fatalf("database file: %v", err)
	}
	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d", fk)
	}
	var journal string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journal).Error; err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q", journal)
	}
}

func TestOpenMemorySQLiteSkipsDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := config.Defaults()
	cfg.Database.DSN = "file:edugo_mem?mode=memory&cache=shared"
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("memory database created files: %v", entries)
	}
}
