package session_test

import (
	"encoding/base32"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/devthinker-ai/factura/pkg/session"
)

func TestTOTP_RFC6238Window(t *testing.T) {
	// Fixed secret (base32, no padding) + known time.
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	now := time.Unix(1111111111, 0).UTC()
	code, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := session.Verify(secret, code, now)
	if err != nil || !ok {
		t.Fatalf("t0: ok=%v err=%v", ok, err)
	}
	ok, _ = session.Verify(secret, code, now.Add(30*time.Second)) // ±1 window
	if !ok {
		t.Fatal("t+1 should accept")
	}
	ok, _ = session.Verify(secret, code, now.Add(-30*time.Second))
	if !ok {
		t.Fatal("t-1 should accept")
	}
	ok, _ = session.Verify(secret, code, now.Add(90*time.Second)) // ±2 rejected
	if ok {
		t.Fatal("t+2 should reject")
	}
	ok, err = session.Verify(secret, "12", now)
	if err != nil || ok {
		t.Fatalf("bad length: ok=%v err=%v", ok, err)
	}
}

func TestGenerateSecret_NoPadding(t *testing.T) {
	s, err := session.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "=") {
		t.Fatalf("padding present: %s", s)
	}
	_, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryCode_LooksLike(t *testing.T) {
	c, err := session.RecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if !session.LooksLikeRecoveryCode(c) {
		t.Fatalf("not recovery shaped: %s", c)
	}
	if session.LooksLikeRecoveryCode("123456") {
		t.Fatal("totp should not look like recovery")
	}
}

type memSettings map[string]string

func (m memSettings) GetSetting(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return v, nil
}
func (m memSettings) SetSetting(key, value string) error {
	m[key] = value
	return nil
}

func TestSession_IssueParse(t *testing.T) {
	t.Setenv("FACTURA_SESSION_SECRET", "test-secret-32-bytes-long!!!!!!")
	mgr := session.MustManager(nil)
	fixed := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	mgr.SetNow(func() time.Time { return fixed })

	tok, err := mgr.Issue(session.User{ID: "u1", Email: "a@b.c", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := mgr.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "u1" || claims.Role != "admin" || claims.Email != "a@b.c" {
		t.Fatalf("%+v", claims)
	}

	mgr.SetNow(func() time.Time { return fixed.Add(25 * time.Hour) })
	if _, err := mgr.Parse(tok); err == nil {
		t.Fatal("25h token should fail")
	}
}

func TestSession_MFATypRejected(t *testing.T) {
	t.Setenv("FACTURA_SESSION_SECRET", "test-secret-32-bytes-long!!!!!!")
	mgr := session.MustManager(nil)
	mfa, err := mgr.IssueMFA("u1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Parse(mfa); err == nil {
		t.Fatal("session Parse must reject mfa typ")
	}
	claims, err := mgr.ParseMFA(mfa)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Typ != "mfa" {
		t.Fatalf("typ=%q", claims.Typ)
	}
	sess, err := mgr.Issue(session.User{ID: "u1", Email: "a@b.c", Role: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ParseMFA(sess); err == nil {
		t.Fatal("ParseMFA must reject session tokens")
	}
}

func TestMFAGate_SingleUse(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	g := session.NewMFAGate(func() time.Time { return now })
	exp := now.Add(5 * time.Minute)
	if !g.MarkUsed("j1", exp) {
		t.Fatal("first mark")
	}
	if g.MarkUsed("j1", exp) {
		t.Fatal("second mark should fail")
	}
	if !g.IsUsed("j1") {
		t.Fatal("should be used")
	}
}

func TestPassword_RoundTrip(t *testing.T) {
	h, err := session.FormatHash("supersecret1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("phc=%s", h)
	}
	if !session.VerifyHash(h, "supersecret1") {
		t.Fatal("verify failed")
	}
	if session.VerifyHash(h, "wrong") {
		t.Fatal("wrong should fail")
	}
}
