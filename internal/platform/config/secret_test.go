package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGeneratesDevJWTSecretWhenUnset(t *testing.T) {
	t.Setenv("EDUGO_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("EDUGO_SERVER_MODE", "debug")
	t.Setenv("EDUGO_AUTH_JWT_SECRET", "")
	t.Setenv("EDUGO_AI_ENABLED", "false")

	first, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(first.Auth.JWTSecret) < 32 {
		t.Fatalf("generated secret too short: %d", len(first.Auth.JWTSecret))
	}
	second, err := Load()
	if err != nil {
		t.Fatalf("load again: %v", err)
	}
	if second.Auth.JWTSecret == "" || second.Auth.JWTSecret == first.Auth.JWTSecret {
		t.Fatal("each process load should mint its own dev secret")
	}
}

func TestLoadKeepsConfiguredJWTSecret(t *testing.T) {
	t.Setenv("EDUGO_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("EDUGO_SERVER_MODE", "debug")
	const secret = "configured-jwt-secret-at-least-32"
	t.Setenv("EDUGO_AUTH_JWT_SECRET", secret)
	t.Setenv("EDUGO_AI_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.JWTSecret != secret {
		t.Fatalf("secret = %q", cfg.Auth.JWTSecret)
	}
}

func TestLoadRejectsEmptyJWTSecretInRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  mode: release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDUGO_CONFIG_PATH", path)
	t.Setenv("EDUGO_SERVER_MODE", "release")
	t.Setenv("EDUGO_AUTH_JWT_SECRET", "")
	t.Setenv("EDUGO_AI_ENABLED", "false")
	t.Setenv("EDUGO_SEED_DEMO", "false")
	t.Setenv("EDUGO_ADMIN_PASSWORD", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT") {
		t.Fatalf("err = %v", err)
	}
}
