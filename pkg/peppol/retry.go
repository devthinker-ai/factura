package peppol

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	retryMu       sync.Mutex
	retryBackoffs = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
)

// SetRetryBackoffsForTest overrides send backoff (tests only).
func SetRetryBackoffsForTest(d []time.Duration) {
	retryMu.Lock()
	defer retryMu.Unlock()
	retryBackoffs = append([]time.Duration(nil), d...)
}

func currentBackoffs() []time.Duration {
	retryMu.Lock()
	defer retryMu.Unlock()
	return append([]time.Duration(nil), retryBackoffs...)
}

// SendWithRetry calls ap.Send with a 30s per-attempt timeout and retries
// transient failures up to 3 times (backoff 2s / 5s / 15s), keyed by the same
// AP message id so sends stay idempotent.
func SendWithRetry(ctx context.Context, ap AccessPoint, m BisMessage) (Receipt, error) {
	if m.Control.APMessageID == "" {
		m.Control.APMessageID = fmt.Sprintf("factura-%d", time.Now().UTC().UnixNano())
	}
	backoffs := currentBackoffs()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := backoffs[0]
			if attempt-1 < len(backoffs) {
				wait = backoffs[attempt-1]
			}
			select {
			case <-ctx.Done():
				return Receipt{}, ctx.Err()
			case <-time.After(wait):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		rec, err := ap.Send(attemptCtx, m)
		cancel()
		if err == nil {
			return rec, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return Receipt{}, lastErr
		}
	}
	return Receipt{}, fmt.Errorf("peppol: ap unreachable after 3 tries: %w", lastErr)
}
