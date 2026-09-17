// Package license validates offline RS256 license JWTs (self-hosted monetization).
//
// Fail-open: invalid/expired/missing keys never brick the box — they fall back
// to Solo caps. No phone-home (air-gapped honor system).
//
// Dep: github.com/golang-jwt/jwt/v5 (the one new Phase-6 Go dependency).
package license

import (
	"crypto/rsa"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

//go:embed license_pub.pem
var pubPEM embed.FS

// Plan names. PlanFree equals PlanSolo — unlicensed boxes run Solo caps.
const (
	PlanFree    = "solo"
	PlanSolo    = "solo"
	PlanPro     = "pro"
	PlanKanzlei = "kanzlei"
)

// Caps are plan limits applied to companies / API keys / Peppol gates.
type Caps struct {
	Plan         string
	Subject      string
	Licensed     bool
	ExpiresAt    time.Time // zero when unlicensed
	MaxCompanies int
	MaxAPIKeys   int
	MaxClients   int // Kanzlei; stored now, consumed in Phase 7
	Peppol       bool
}

// Claims is the JWT payload for a signed license key.
type Claims struct {
	Plan         string `json:"plan"`
	MaxCompanies int    `json:"max_companies"`
	MaxAPIKeys   int    `json:"max_api_keys"`
	MaxClients   int    `json:"max_clients"`
	jwt.RegisteredClaims
}

// SettingsReader looks up a persisted setting (settings.license_key).
type SettingsReader interface {
	GetSetting(key string) (string, error)
}

// FreeCaps returns unlicensed Solo-tier limits (fail-open default).
func FreeCaps() Caps {
	c := CapsForPlan(PlanSolo)
	c.Licensed = false
	c.Subject = ""
	c.ExpiresAt = time.Time{}
	return c
}

// CapsForPlan maps plan names to default caps (claim overrides via pick).
func CapsForPlan(plan string) Caps {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case PlanPro:
		return Caps{
			Plan: PlanPro, MaxCompanies: 10, MaxAPIKeys: 20, MaxClients: 10, Peppol: true,
		}
	case PlanKanzlei:
		return Caps{
			Plan: PlanKanzlei, MaxCompanies: 50, MaxAPIKeys: 50, MaxClients: 50, Peppol: true,
		}
	default:
		// solo + unknown → Solo defaults
		return Caps{
			Plan: PlanSolo, MaxCompanies: 1, MaxAPIKeys: 2, MaxClients: 0, Peppol: false,
		}
	}
}

// Validate verifies RS256 signature with the embedded public key and checks exp.
func Validate(token string) (Claims, error) {
	pub, err := loadPublicKey()
	if err != nil {
		return Claims{}, err
	}
	return ValidateWithKey(token, pub)
}

// ValidateWithKey is used by tests (ephemeral keys) and Validate.
func ValidateWithKey(token string, pub *rsa.PublicKey) (Claims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Claims{}, errors.New("empty license key")
	}
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected alg %s", t.Method.Alg())
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil {
		return Claims{}, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return Claims{}, errors.New("invalid claims")
	}
	if claims.Plan == "" {
		claims.Plan = PlanSolo
	}
	return *claims, nil
}

// Sign creates a license JWT (CLI / tests). Private key never ships in the binary.
func Sign(priv *rsa.PrivateKey, claims Claims) (string, error) {
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(time.Now().UTC())
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().UTC().Add(365 * 24 * time.Hour))
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return t.SignedString(priv)
}

// Resolve reads key from arg, else FACTURA_LICENSE_KEY, else settings.license_key.
// Fail-opens to FreeCaps on missing/invalid/expired.
func Resolve(log *slog.Logger, key string, settings SettingsReader) Caps {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(key) == "" {
		key = os.Getenv("FACTURA_LICENSE_KEY")
	}
	if strings.TrimSpace(key) == "" && settings != nil {
		if v, err := settings.GetSetting("license_key"); err == nil {
			key = v
		}
	}
	if strings.TrimSpace(key) == "" {
		caps := FreeCaps()
		log.Warn("Factura — UNLICENSED — Solo caps (1 company, no Peppol). Set FACTURA_LICENSE_KEY or paste a key in Settings.")
		return caps
	}
	claims, err := Validate(key)
	if err != nil {
		log.Error("LICENSE KEY INVALID OR EXPIRED — falling back to Solo caps", "err", err)
		return FreeCaps()
	}
	return capsFromClaims(log, claims)
}

// ResolveWithKey is Resolve for tests (ephemeral RSA keys) — same fail-open rules.
func ResolveWithKey(log *slog.Logger, key string, pub *rsa.PublicKey) Caps {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(key) == "" {
		return FreeCaps()
	}
	claims, err := ValidateWithKey(key, pub)
	if err != nil {
		log.Error("LICENSE KEY INVALID OR EXPIRED — falling back to Solo caps", "err", err)
		return FreeCaps()
	}
	return capsFromClaims(log, claims)
}

// CapsFromClaims builds Caps from validated claims (no logging).
func CapsFromClaims(claims Claims) Caps {
	base := CapsForPlan(claims.Plan)
	caps := Caps{
		Plan:         strings.ToLower(claims.Plan),
		Subject:      claims.Subject,
		MaxCompanies: pick(claims.MaxCompanies, base.MaxCompanies),
		MaxAPIKeys:   pick(claims.MaxAPIKeys, base.MaxAPIKeys),
		MaxClients:   pick(claims.MaxClients, base.MaxClients),
		Peppol:       base.Peppol,
		Licensed:     true,
	}
	if claims.ExpiresAt != nil {
		caps.ExpiresAt = claims.ExpiresAt.Time
	}
	return caps
}

func capsFromClaims(log *slog.Logger, claims Claims) Caps {
	caps := CapsFromClaims(claims)
	exp := ""
	if !caps.ExpiresAt.IsZero() {
		exp = caps.ExpiresAt.UTC().Format(time.RFC3339)
	}
	log.Info("Factura — licensed",
		"plan", caps.Plan,
		"subject", caps.Subject,
		"expires", exp,
	)
	return caps
}

func pick(n, fallback int) int {
	if n > 0 {
		return n
	}
	return fallback
}

func loadPublicKey() (*rsa.PublicKey, error) {
	b, err := pubPEM.ReadFile("license_pub.pem")
	if err != nil {
		return nil, err
	}
	return ParseRSAPublicKey(b)
}
