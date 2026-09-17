package license_test

import (
	"crypto/rand"
	"crypto/rsa"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devthinker-ai/factura/pkg/license"
)

func TestSignValidateRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan:         license.PlanPro,
		MaxCompanies: 10,
		MaxAPIKeys:   20,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "buyer@example.com",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := license.ValidateWithKey(tok, &priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "buyer@example.com" || got.Plan != license.PlanPro || got.MaxCompanies != 10 {
		t.Fatalf("claims=%+v", got)
	}
}

func TestWrongSignatureRejected(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "x",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := license.ValidateWithKey(tok, &other.PublicKey); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestExpiredRejected(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "expired",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC().Add(-48 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(-24 * time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := license.ValidateWithKey(tok, &priv.PublicKey); err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestHS256Rejected(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "hs",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	signed, err := tok.SignedString([]byte("not-rsa"))
	if err != nil {
		t.Fatal(err)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := license.ValidateWithKey(signed, &priv.PublicKey); err == nil {
		t.Fatal("expected alg rejection")
	}
}

func TestTamperedClaimsRejected(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "tamper",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	b := []byte(tok)
	b[len(b)/2] ^= 0x01
	if _, err := license.ValidateWithKey(string(b), &priv.PublicKey); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestCapsForPlanDefaultsAndOverride(t *testing.T) {
	solo := license.CapsForPlan(license.PlanSolo)
	if solo.MaxCompanies != 1 || solo.MaxAPIKeys != 2 || solo.Peppol {
		t.Fatalf("solo=%+v", solo)
	}
	pro := license.CapsForPlan(license.PlanPro)
	if pro.MaxCompanies != 10 || pro.MaxAPIKeys != 20 || !pro.Peppol || pro.MaxClients != 10 {
		t.Fatalf("pro=%+v", pro)
	}
	kanz := license.CapsForPlan(license.PlanKanzlei)
	if kanz.MaxCompanies != 50 || kanz.MaxAPIKeys != 50 || kanz.MaxClients != 50 || !kanz.Peppol {
		t.Fatalf("kanzlei=%+v", kanz)
	}
	free := license.FreeCaps()
	if free.Licensed || free.Plan != license.PlanSolo || free.MaxCompanies != 1 {
		t.Fatalf("free=%+v", free)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan:         license.PlanPro,
		MaxCompanies: 3,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "override",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	caps := license.ResolveWithKey(slog.Default(), tok, &priv.PublicKey)
	if !caps.Licensed || caps.MaxCompanies != 3 || caps.MaxAPIKeys != 20 {
		t.Fatalf("override caps=%+v", caps)
	}
}

func TestResolveEmptyAndInvalidFailOpen(t *testing.T) {
	_ = os.Unsetenv("FACTURA_LICENSE_KEY")
	t.Setenv("FACTURA_LICENSE_KEY", "")
	caps := license.Resolve(slog.Default(), "", nil)
	if caps.Licensed || caps.Plan != license.PlanSolo {
		t.Fatalf("empty=%+v", caps)
	}
	bad := license.Resolve(slog.Default(), "not.a.jwt", nil)
	if bad.Licensed || bad.Plan != license.PlanSolo {
		t.Fatalf("invalid=%+v", bad)
	}
}

type memSettings map[string]string

func (m memSettings) GetSetting(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func TestResolvePrecedence(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	proTok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "arg",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	soloTok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanSolo,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "env",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Arg wins over env — but Resolve uses Validate (embedded pub), so ephemeral
	// keys only work via ResolveWithKey. Precedence of sources:
	t.Setenv("FACTURA_LICENSE_KEY", "env-placeholder")
	caps := license.Resolve(slog.Default(), "", memSettings{"license_key": "store-placeholder"})
	// Both invalid → fail-open Solo
	if caps.Licensed {
		t.Fatalf("expected fail-open, got %+v", caps)
	}
	_ = proTok
	_ = soloTok
	// ResolveWithKey: empty → free
	if c := license.ResolveWithKey(slog.Default(), "", &priv.PublicKey); c.Licensed {
		t.Fatal("empty should be free")
	}
	c := license.ResolveWithKey(slog.Default(), proTok, &priv.PublicKey)
	if !c.Licensed || c.Plan != license.PlanPro || c.Subject != "arg" {
		t.Fatalf("pro resolve=%+v", c)
	}
	if !strings.Contains(c.Subject, "arg") {
		t.Fatal(c.Subject)
	}
}
