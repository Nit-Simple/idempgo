package idempgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ConflictPolicy int

const (
	ConflictReject ConflictPolicy = iota + 1
	ConflictBlock
)

type Config struct {
	KeyExtractor        KeyExtractor
	AllowDerivedKeys    bool
	GenerateFingerprint GenerateFingerprint

	// --TTl
	ExplicitKeyTTL time.Duration // default: 24h
	DerivedKeyTTL  time.Duration // default: 5min
	InFlightTTL    time.Duration // default: 30s -- in flight marker expiry
	ErrorTTL       time.Duration // default: sets to same Duration in 10mins

	//concurrency
	ConflictPolicy ConflictPolicy // default: reject(409)
	LockTimeout    time.Duration  // default: 0 -- only required if ConflictPolicy == Block

	// --Response--
	ShowReplayHeader       bool
	ReplayHeader           string
	StoreErrorCodes        []int
	StoreErrors            bool
	MaxStoredResponseBytes int64
}

func withDefaults(cfg *Config) *Config {

	if cfg.ExplicitKeyTTL == 0 {
		cfg.ExplicitKeyTTL = 24 * time.Hour
	}
	if cfg.DerivedKeyTTL == 0 {
		cfg.DerivedKeyTTL = 5 * time.Minute
	}
	if cfg.InFlightTTL == 0 {
		cfg.InFlightTTL = 30 * time.Second
	}
	if cfg.ErrorTTL == 0 {
		cfg.ErrorTTL = 10 * time.Minute
	}
	if cfg.MaxStoredResponseBytes == 0 {
		cfg.MaxStoredResponseBytes = 64 * 1024 // 64KB
	}
	if cfg.ReplayHeader == "" {
		cfg.ReplayHeader = "Idempotency-Replayed"
	}
	return cfg
}

func validate(cfg *Config) error {
	if cfg.KeyExtractor == nil {
		return errors.New("idempotency: KeyExtractor is required")
	}
	if cfg.AllowDerivedKeys && cfg.GenerateFingerprint == nil {
		return errors.New("idempotency: GenerateFingerprint is required when AllowDerivedKeys is true")
	}
	if cfg.ConflictPolicy == ConflictBlock && cfg.LockTimeout == 0 {
		return errors.New("idempotency: LockTimeout must be set when ConflictPolicy is Block")
	}
	if cfg.StoreErrorCodes != nil && !cfg.StoreErrors {
		return errors.New("idempotency: StoreErrorCodes has no effect when StoreErrors is false")
	}
	if cfg.ShowReplayHeader && cfg.ReplayHeader == "" {
		// can't happen after withDefaults, but guard anyway
		return errors.New("idempotency: ReplayHeader cannot be empty when ShowReplayHeader is true")
	}
	// TTL sanity checks
	if cfg.InFlightTTL > cfg.ExplicitKeyTTL {
		return errors.New("idempotency: InFlightTTL should not exceed ExplicitKeyTTL")
	}
	if cfg.LockTimeout > cfg.InFlightTTL {
		return errors.New("idempotency: LockTimeout exceeds InFlightTTL -- lock will expire before request completes")
	}
	return nil
}

func New(store Store, cfg *Config) (*Middleware, error) {
	cfg = withDefaults(cfg)
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return &Middleware{store: store, cfg: cfg}, nil
}

type KeyExtractor func(ctx context.Context, r *http.Request) (key string, err error)

type GenerateFingerprint func(ctx context.Context, r *http.Request) (string, error)

func defaultKeyExtractor(ctx context.Context, r *http.Request) (key string, err error) {
	key = r.Header.Get("X-Idempotency-Key")
	if key == "" {
		return "", errors.New("X-Idempotency-key header is empty ")
	}
	return strings.ToLower(key), nil

}

func defaultGenerateFingerprint(ctx context.Context, r *http.Request) (string, error) {

	if r.GetBody == nil {
		return "", fmt.Errorf("fingerprint: request body is not rewindable ")
	}
	bodyReader, err := r.GetBody()
	if err != nil {
		return "", fmt.Errorf("fingerprint: getting body : %w", err)
	}
	defer bodyReader.Close()
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return "", fmt.Errorf("fingerprint: reading body : %w", err)
	}
	h := sha256.New()
	h.Write([]byte(r.Method))
	h.Write([]byte{0})
	h.Write([]byte(r.URL.Path))
	h.Write([]byte{0})
	h.Write(body)

	return hex.EncodeToString(h.Sum(nil)), nil

}

// genarates a sha256 fingerprint using path method  and scope it takes in a scope function to compute the scope and returns the finger print func
func PathOnlyFingerprint(scopeFunc func(ctx context.Context, r *http.Request) string) GenerateFingerprint {
	return func(ctx context.Context, r *http.Request) (string, error) {
		scope := scopeFunc(ctx, r)

		h := sha256.New()
		h.Write([]byte(r.Method))
		h.Write([]byte{0})
		h.Write([]byte(r.URL.Path))
		h.Write([]byte{0})
		h.Write([]byte(scope))

		return hex.EncodeToString(h.Sum(nil)), nil
	}
}
