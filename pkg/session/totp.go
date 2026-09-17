package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
)

// Recovery alphabet: unambiguous (no 0/O/1/l/I).
const recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789abcdefghjkmnpqrstuvwxyz"

const (
	totpDigits       = 6
	totpPeriod       = 30
	secretBytes      = 20
	recoveryLen      = 8
	recoveryCount    = 10
	issuerName       = "Factura"
	mfaMaxAttempts   = 5
	mfaRetryAfterSec = 60
)

// MFAMaxAttempts / MFARetryAfterSec exported for handlers and tests.
const (
	MFAMaxAttempts   = mfaMaxAttempts
	MFARetryAfterSec = mfaRetryAfterSec
)

// GenerateSecret returns a 20-byte TOTP secret as base32 RFC 4648 uppercase, no padding.
func GenerateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// ProvisioningURI builds otpauth://totp/Factura:<email>?secret=…&issuer=Factura&digits=6&period=30.
func ProvisioningURI(secret, email string) string {
	label := issuerName + ":" + email
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuerName)
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", totpPeriod))
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}

// QRDataURL returns a PNG data URL (~240 px) for the provisioning URI.
func QRDataURL(uri string) (string, error) {
	png, err := qrcode.Encode(uri, qrcode.Medium, 240)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// Verify checks a 6-digit TOTP code against secret at now (±1 window).
// Wrong length / non-digits → false, nil (not an error).
func Verify(secret, code string, now time.Time) (bool, error) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false, nil
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false, nil
		}
	}
	ok, err := totp.ValidateCustom(code, secret, now.UTC(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, nil
	}
	return ok, nil
}

// RecoveryCode returns one plaintext recovery code in xxxx-xxxx form.
func RecoveryCode() (string, error) {
	b := make([]byte, recoveryLen)
	max := big.NewInt(int64(len(recoveryAlphabet)))
	for i := 0; i < recoveryLen; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = recoveryAlphabet[n.Int64()]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

// GenerateRecoveryCodes returns recoveryCount plaintext codes.
func GenerateRecoveryCodes() ([]string, error) {
	out := make([]string, recoveryCount)
	for i := 0; i < recoveryCount; i++ {
		c, err := RecoveryCode()
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

// HashRecoveryCode returns sha256 hex of the trimmed code.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

// LooksLikeRecoveryCode reports whether code matches xxxx-xxxx.
func LooksLikeRecoveryCode(code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 9 || code[4] != '-' {
		return false
	}
	for i, c := range code {
		if i == 4 {
			continue
		}
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '2' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// MFAGate tracks single-use jti + attempt counters for mfa_tokens (in-memory).
type MFAGate struct {
	mu       sync.Mutex
	used     map[string]time.Time // jti → exp
	attempts map[string]int
	now      func() time.Time
}

// NewMFAGate creates an empty gate. Optional now injector for tests.
func NewMFAGate(now func() time.Time) *MFAGate {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &MFAGate{
		used:     make(map[string]time.Time),
		attempts: make(map[string]int),
		now:      now,
	}
}

// SetNow updates the clock.
func (g *MFAGate) SetNow(fn func() time.Time) {
	if fn == nil {
		return
	}
	g.mu.Lock()
	g.now = fn
	g.mu.Unlock()
}

func (g *MFAGate) purge(now time.Time) {
	for jti, exp := range g.used {
		if now.After(exp) {
			delete(g.used, jti)
			delete(g.attempts, jti)
		}
	}
}

// CheckAndCountAttempt increments the per-jti counter. On 6th attempt returns false.
func (g *MFAGate) CheckAndCountAttempt(jti string, exp time.Time) (allowed bool, n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.purge(now)
	if !exp.After(now) {
		return false, mfaMaxAttempts
	}
	g.attempts[jti]++
	n = g.attempts[jti]
	if n > mfaMaxAttempts {
		return false, n
	}
	return true, n
}

// MarkUsed records jti as consumed until exp. Returns false if already used or expired.
func (g *MFAGate) MarkUsed(jti string, exp time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.purge(now)
	if !exp.After(now) {
		return false
	}
	if _, ok := g.used[jti]; ok {
		return false
	}
	g.used[jti] = exp
	return true
}

// IsUsed reports whether jti was already consumed.
func (g *MFAGate) IsUsed(jti string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.purge(now)
	_, ok := g.used[jti]
	return ok
}

// LoginLimiter rate-limits failed POST /login per email (10 / 15 min).
type LoginLimiter struct {
	mu       sync.Mutex
	fails    map[string][]time.Time
	now      func() time.Time
	maxFails int
	window   time.Duration
}

const (
	loginMaxFails = 10
	loginWindow   = 15 * time.Minute
)

// NewLoginLimiter creates a per-email brute-force gate.
func NewLoginLimiter(now func() time.Time) *LoginLimiter {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &LoginLimiter{
		fails:    make(map[string][]time.Time),
		now:      now,
		maxFails: loginMaxFails,
		window:   loginWindow,
	}
}

// SetNow updates the clock.
func (l *LoginLimiter) SetNow(fn func() time.Time) {
	if fn == nil {
		return
	}
	l.mu.Lock()
	l.now = fn
	l.mu.Unlock()
}

// Allowed reports whether another attempt is permitted for email.
func (l *LoginLimiter) Allowed(email string) (ok bool, retryAfterSec int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	now := l.now()
	l.prune(email, now)
	if len(l.fails[email]) >= l.maxFails {
		oldest := l.fails[email][0]
		retry := int(l.window.Seconds() - now.Sub(oldest).Seconds())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	return true, 0
}

// RecordFail records a failed login for email.
func (l *LoginLimiter) RecordFail(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	now := l.now()
	l.prune(email, now)
	l.fails[email] = append(l.fails[email], now)
}

// Clear resets the fail window for email (successful login).
func (l *LoginLimiter) Clear(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, strings.ToLower(strings.TrimSpace(email)))
}

func (l *LoginLimiter) prune(email string, now time.Time) {
	cut := now.Add(-l.window)
	var kept []time.Time
	for _, t := range l.fails[email] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, email)
	} else {
		l.fails[email] = kept
	}
}
