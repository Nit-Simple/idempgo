package idempgo

import "errors"

var (
	ErrFailToDeriveKey = errors.New("Failed to derive key")
	ErrTimeOut         = errors.New("Inflight TTL time out exceeded ")
	ErrMissingKeys     = errors.New("Missing idempotency key")
	ErrNilRecord       = errors.New("The received method was nil")
	ErrBlockTimeOut    = errors.New("LockTTL time out exceeded")
)
