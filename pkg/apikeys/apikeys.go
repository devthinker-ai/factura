// Package apikeys validates machine factura_* API keys and enforces RPM + monthly quota.
package apikeys

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devthinker-ai/factura/pkg/archive"
)

const keyPrefix = "factura_"

// CachedKey is the in-memory view used on the auth hot path.
type CachedKey struct {
	ID                string
	Name              string
	RPM               int
	MonthlyInvoices   int
	InvoicesThisMonth int
	Period            string
	Enabled           bool
	KeyHash           string
}

// Validator authenticates Bearer factura_* keys and enforces kill/RPM/quota.
type Validator struct {
	store *archive.Store

	mu    sync.RWMutex
	cache map[string]*CachedKey // keyHash → key

	rateMu sync.Mutex
	rate   map[string][]time.Time

	now func() time.Time
}

// NewValidator loads keys from the store into an in-memory cache.
func NewValidator(s *archive.Store) (*Validator, error) {
	v := &Validator{
		store: s,
		cache: make(map[string]*CachedKey),
		rate:  make(map[string][]time.Time),
		now:   func() time.Time { return time.Now().UTC() },
	}
	if err := v.Refresh(); err != nil {
		return nil, err
	}
	return v, nil
}

// SetNow injects a clock (tests).
func (v *Validator) SetNow(fn func() time.Time) {
	if fn != nil {
		v.now = fn
	}
}

// Refresh reloads the in-memory cache from SQLite.
func (v *Validator) Refresh() error {
	keys, err := v.store.ListKeys()
	if err != nil {
		return err
	}
	period := v.now().Format("2006-01")
	next := make(map[string]*CachedKey, len(keys))
	for _, k := range keys {
		used := k.InvoicesThisMonth
		if k.Period != period {
			used = 0
		}
		ck := &CachedKey{
			ID:                k.ID,
			Name:              k.Name,
			RPM:               k.RPM,
			MonthlyInvoices:   k.MonthlyInvoices,
			InvoicesThisMonth: used,
			Period:            period,
			Enabled:           k.Enabled,
			KeyHash:           k.KeyHash,
		}
		next[k.KeyHash] = ck
	}
	v.mu.Lock()
	v.cache = next
	v.mu.Unlock()
	return nil
}

type ctxKey int

const apiKeyCtx ctxKey = 1

// KeyFromContext returns the CachedKey attached by auth middleware.
func KeyFromContext(ctx context.Context) (*CachedKey, bool) {
	k, ok := ctx.Value(apiKeyCtx).(*CachedKey)
	return k, ok
}

// WithKey attaches a CachedKey to the request context.
func WithKey(ctx context.Context, k *CachedKey) context.Context {
	return context.WithValue(ctx, apiKeyCtx, k)
}

// IsAPIKeyToken reports whether the bearer looks like a machine key.
func IsAPIKeyToken(token string) bool {
	return strings.HasPrefix(token, keyPrefix)
}

// CheckResult is the outcome of Validator.Check.
type CheckResult int

const (
	CheckOK CheckResult = iota
	CheckUnauthorized
	CheckRateLimited
	CheckQuotaExceeded
)

// Check authenticates a factura_* bearer token.
// If checkQuota is true, also rejects when the monthly invoice cap is reached.
func (v *Validator) Check(token string, checkQuota bool) (*CachedKey, CheckResult, string) {
	token = strings.TrimSpace(token)
	if token == "" || !IsAPIKeyToken(token) {
		return nil, CheckUnauthorized, ""
	}
	hash := archive.HashAPIKey(token)

	v.mu.RLock()
	cached, ok := v.cache[hash]
	v.mu.RUnlock()
	if !ok || !cached.Enabled {
		return nil, CheckUnauthorized, ""
	}

	if cached.RPM > 0 {
		if retryAfter, limited := v.checkRate(cached.ID, cached.RPM); limited {
			return nil, CheckRateLimited, retryAfter
		}
	}

	if checkQuota && cached.MonthlyInvoices > 0 {
		period := v.now().Format("2006-01")
		used := cached.InvoicesThisMonth
		if cached.Period != period {
			used = 0
		}
		if used >= cached.MonthlyInvoices {
			return nil, CheckQuotaExceeded, ""
		}
	}

	return cached, CheckOK, ""
}

// RecordUsage increments store + in-memory counters after a successful invoice op.
func (v *Validator) RecordUsage(keyID string) error {
	used, err := v.store.IncrementUsage(keyID, v.now)
	if err != nil {
		return err
	}
	period := v.now().Format("2006-01")
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, k := range v.cache {
		if k.ID == keyID {
			k.InvoicesThisMonth = used
			k.Period = period
			break
		}
	}
	return nil
}

func (v *Validator) checkRate(keyID string, rpm int) (retryAfter string, limited bool) {
	now := v.now()
	windowStart := now.Add(-60 * time.Second)

	v.rateMu.Lock()
	defer v.rateMu.Unlock()

	stamps := v.rate[keyID]
	kept := stamps[:0]
	for _, t := range stamps {
		if t.After(windowStart) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rpm {
		v.rate[keyID] = kept
		return "1", true
	}
	v.rate[keyID] = append(kept, now)
	return "", false
}

// WriteUnauthorized writes the shared 401 body.
func WriteUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}` + "\n"))
}

// WriteRateLimited writes 429 + Retry-After.
func WriteRateLimited(w http.ResponseWriter, retryAfter string) {
	if retryAfter == "" {
		retryAfter = "1"
	}
	w.Header().Set("Retry-After", retryAfter)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"rate limited"}` + "\n"))
}

// WriteQuotaExceeded writes 403 quota_exceeded.
func WriteQuotaExceeded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"quota_exceeded"}` + "\n"))
}

// Itoa is a tiny helper for Retry-After.
func Itoa(n int) string { return strconv.Itoa(n) }
