// Package session issues HS256 console JWTs and verifies TOTP MFA.
//
// Machine keys (factura_*) and the operator FACTURA_TOKEN never go through
// this package. Sessions are stateless — a removed user's token stays valid
// until SessionTTL (24 h); that is a known v1 trade-off (see AGENTS.md).
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	issuer     = "factura"
	SessionTTL = 24 * time.Hour
	mfaTTL     = 5 * time.Minute
	typMFA     = "mfa"
	typSession = "" // empty typ = session token
)

// ErrInvalidToken is returned when a JWT fails signature/exp/typ checks.
var ErrInvalidToken = errors.New("session: invalid token")

// Claims are embedded in HS256 session / MFA pre-tokens.
// MFA pre-tokens set Typ="mfa"; session Parse rejects them and vice versa.
type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email"`
	Typ   string `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

// User is the minimal identity needed to issue a session.
type User struct {
	ID    string
	Email string
	Role  string
	Name  string
}

// SettingsStore reads/writes the auth_session_secret KV.
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// Manager issues and validates console session JWTs.
type Manager struct {
	secret []byte
	now    func() time.Time
}

// NewManager resolves the signing secret:
// FACTURA_SESSION_SECRET env → else settings.auth_session_secret → else generate
// 32 random bytes, base64, persist via SetSetting.
func NewManager(store SettingsStore) (*Manager, error) {
	secret := strings.TrimSpace(os.Getenv("FACTURA_SESSION_SECRET"))
	if secret == "" && store != nil {
		v, err := store.GetSetting("auth_session_secret")
		if err == nil && v != "" {
			secret = v
		} else {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return nil, err
			}
			secret = base64.StdEncoding.EncodeToString(b)
			if err := store.SetSetting("auth_session_secret", secret); err != nil {
				return nil, err
			}
		}
	}
	if secret == "" {
		return nil, errors.New("session: no signing secret")
	}
	return &Manager{
		secret: []byte(secret),
		now:    func() time.Time { return time.Now().UTC() },
	}, nil
}

// MustManager panics on error (tests).
func MustManager(store SettingsStore) *Manager {
	m, err := NewManager(store)
	if err != nil {
		panic(err)
	}
	return m
}

// SetNow injects a clock (tests / MFA window).
func (m *Manager) SetNow(fn func() time.Time) {
	if fn != nil {
		m.now = fn
	}
}

func (m *Manager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now().UTC()
}

// Issue returns a 24 h session JWT for the user.
func (m *Manager) Issue(user User) (string, error) {
	return m.issue(user, typSession, SessionTTL)
}

// IssueMFA returns a 5-minute pre-token (typ=mfa) after password OK + TOTP enrolled.
func (m *Manager) IssueMFA(userID string) (string, error) {
	return m.issue(User{ID: userID}, typMFA, mfaTTL)
}

func (m *Manager) issue(user User, typ string, ttl time.Duration) (string, error) {
	now := m.clock()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	claims := Claims{
		Role:  user.Role,
		Email: user.Email,
		Typ:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   user.ID,
			ID:        hex.EncodeToString(jti),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(m.secret)
}

// Parse validates a session token. Rejects typ=mfa and expired/bad signatures.
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	claims, err := m.parseRaw(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Typ == typMFA {
		return nil, ErrInvalidToken
	}
	if claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ParseMFA validates an MFA pre-token (requires typ=mfa). Does not enforce single-use.
func (m *Manager) ParseMFA(tokenStr string) (*Claims, error) {
	claims, err := m.parseRaw(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Typ != typMFA {
		return nil, ErrInvalidToken
	}
	if claims.Subject == "" || claims.ID == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *Manager) parseRaw(tokenStr string) (*Claims, error) {
	now := m.clock()
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Issuer != issuer {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

type ctxKey int

const authUserKey ctxKey = 1

// AuthUser is attached to the request context for session logins.
type AuthUser struct {
	ID    string
	Email string
	Role  string
	Name  string
}

// WithUser returns a context carrying AuthUser.
func WithUser(ctx context.Context, u AuthUser) context.Context {
	return context.WithValue(ctx, authUserKey, u)
}

// UserFromContext returns the session user, if any.
func UserFromContext(ctx context.Context) (AuthUser, bool) {
	u, ok := ctx.Value(authUserKey).(AuthUser)
	return u, ok
}
