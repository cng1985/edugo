package authn

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/golang-jwt/jwt/v5"
)

type claims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func SignAccess(secret, userID, sessionID, issuer string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		UserID: userID, SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Audience: []string{"edugo"}, Subject: userID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return token.SignedString([]byte(secret))
}

func ParseAccess(secret, raw string) (userID, sessionID string, err error) {
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, apperr.Unauthorized("令牌算法无效")
		}
		return []byte(secret), nil
	}, jwt.WithAudience("edugo"))
	if err != nil {
		return "", "", apperr.Unauthorized("访问令牌无效或已过期")
	}
	c, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid || c.UserID == "" || c.SessionID == "" {
		return "", "", apperr.Unauthorized("访问令牌无效")
	}
	return c.UserID, c.SessionID, nil
}

func NewRefresh() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain := base64.RawURLEncoding.EncodeToString(buf)
	return plain, Hash(plain), nil
}

func Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
