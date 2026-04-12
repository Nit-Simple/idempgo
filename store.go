package idempgo

import (
	"context"
	"time"
)

type Store interface {
	// CreateOrGet atomically checks for an existing record and either:
	// - returns it if found (IN_PROGRESS or COMPLETE)
	// - creates a new IN_PROGRESS record and returns acquired=true if not found
	// This MUST be atomic — it's the core correctness guarantee of the library
	CreateOrGet(ctx context.Context, key string, fingerPrint string, lockTTL time.Duration) (record *IdempotencyRecord, acquired bool, err error)

	Get(ctx context.Context, key string) (record *IdempotencyRecord, err error)
	// Complete transitions an IN_PROGRESS record to COMPLETE and stores the response
	// ttl is the remaining lifetime of this record from now
	Complete(ctx context.Context, key string, response *StoredResponse, ttl time.Duration) error

	// Abandon releases an in-flight key without completing it, allowing the next
	// retry to re-acquire it. Call this when the upstream handler returns a 5xx —
	// if you don't, the key stays locked until InFlightTTL expires and every
	// retry from the client gets a 409 Conflict instead of being processed.
	Abandon(ctx context.Context, key string) error
}
