package idempgo

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

type Middleware struct {
	store Store
	cfg   *Config
}
type ResponseRecorder struct {
	http.ResponseWriter
	StatusCode int
	body       bytes.Buffer
	header     http.Header
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
	r.StatusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
func (r *ResponseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (m *Middleware) AcquireOrGet(ctx context.Context, inflight time.Duration, key string, fingerPrint string) (outcome AcquireOutcome, err error) {
	record, acquired, err := m.store.CreateOrGet(ctx, key, fingerPrint, inflight)
	if err != nil {
		return AcquireOutcome{}, err
	}
	if acquired {
		return AcquireOutcome{Result: Acquired}, nil
	}
	switch record.Status {
	case STATUS_COMPLETED:
		return AcquireOutcome{Result: Replayed, Record: record}, nil
	case STATUS_IN_FLIGHT:
		return AcquireOutcome{Result: Conflict}, nil
	}
	return AcquireOutcome{Result: Conflict}, nil

}
func (m *Middleware) blockUntillResolved(ctx context.Context, key string, inflght time.Duration, fingerPrint string) (outcome AcquireOutcome, err error) {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.LockTimeout)
	defer cancel()

	interval := 50 * time.Millisecond
	max := 500 * time.Millisecond
	for {
		record, err := m.store.Get(ctx, key)
		if err != nil {
			return AcquireOutcome{}, err
		}
		if record == nil {
			outcome, err := m.AcquireOrGet(ctx, inflght, key, fingerPrint)
			if err != nil {
				return AcquireOutcome{}, err
			}
			if outcome.Result != Conflict {
				return outcome, nil // Acquired or Replayed — done
			}
			// another goroutine beat us, keep polling
		} else if record.Status == STATUS_COMPLETED {
			return AcquireOutcome{Result: Replayed, Record: record}, nil
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return AcquireOutcome{}, ErrBlockTimeOut
		}
		interval = min(interval*2, max)
	}
}

func (m *Middleware) resolveKey(ctx context.Context, r *http.Request) (resolvedKey ResolvedKey, err error) {
	key, err := m.cfg.KeyExtractor(ctx, r)
	if err != nil {
		return ResolvedKey{}, err
	}
	if key != "" {
		return ResolvedKey{Key: key, Derived: false}, nil
	}
	if !m.cfg.AllowDerivedKeys {
		return ResolvedKey{}, ErrMissingKeys
	}

	derivedkey, err := m.cfg.GenerateFingerprint(ctx, r)

	if err != nil {
		return ResolvedKey{}, ErrFailToDeriveKey
	}

	if derivedkey == "" {
		return ResolvedKey{}, ErrMissingKeys
	}
	return ResolvedKey{Key: derivedkey, Derived: true}, nil
}
