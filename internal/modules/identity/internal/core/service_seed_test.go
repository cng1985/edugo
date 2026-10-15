package core

import (
	"context"
	"testing"

	"github.com/cng1985/edugo/internal/modules/identity/api"
	"github.com/cng1985/edugo/internal/modules/identity/internal/store"
	"github.com/cng1985/edugo/internal/platform/audit"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDebugModeSeedsDefaultAdmin(t *testing.T) {
	svc := newSeedService(t, "debug", "")
	if hint := svc.BootstrapHint(); hint == nil || hint.Username != "admin" || hint.Password != "admin123" {
		t.Fatalf("hint: %+v", hint)
	}
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), api.LoginRequest{Username: "admin", Password: "admin123"}); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseModeDoesNotSeedDefaultAdmin(t *testing.T) {
	svc := newSeedService(t, "release", "")
	if svc.BootstrapHint() != nil {
		t.Fatal("release hinted a default password")
	}
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), api.LoginRequest{Username: "admin", Password: "admin123"}); err == nil {
		t.Fatal("release created the debug admin")
	}
}

func TestConfiguredPasswordReplacesExistingAdmin(t *testing.T) {
	svc := newSeedService(t, "debug", "AdminPass1")
	if svc.BootstrapHint() != nil {
		t.Fatal("configured password should not be hinted")
	}
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.cfg.Admin.Password = "AdminPass2"
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), api.LoginRequest{Username: "admin", Password: "AdminPass2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), api.LoginRequest{Username: "admin", Password: "AdminPass1"}); err == nil {
		t.Fatal("old password still works")
	}
}

func TestDebugDefaultDoesNotReplaceCustomPassword(t *testing.T) {
	svc := newSeedService(t, "debug", "AdminPass1")
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.cfg.Admin.Password = ""
	if err := svc.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), api.LoginRequest{Username: "admin", Password: "AdminPass1"}); err != nil {
		t.Fatal(err)
	}
}

func newSeedService(t *testing.T, mode, password string) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(append(store.Models(), audit.Models()...)...); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Server.Mode = mode
	cfg.Admin.Password = password
	cfg.Security.BcryptCost = 4
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return New(store.New(db), cfg, audit.New(db))
}
