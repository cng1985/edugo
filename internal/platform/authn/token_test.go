package authn

import (
	"testing"
	"time"

	"github.com/cng1985/edugo/internal/shared/apperr"
)

func TestSignAndParseAccess(t *testing.T) {
	const secret = "integration-test-secret-32chars-min"
	raw, err := SignAccess(secret, "user-1", "sess-1", "edugo", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uid, sid, err := ParseAccess(secret, raw)
	if err != nil {
		t.Fatal(err)
	}
	if uid != "user-1" || sid != "sess-1" {
		t.Fatalf("claims = %s %s", uid, sid)
	}
	_, _, err = ParseAccess("different-secret-different-secret", raw)
	if err == nil || apperr.As(err).Msg != "访问令牌无效或已过期" {
		t.Fatalf("mismatched secret err = %v", err)
	}
}
