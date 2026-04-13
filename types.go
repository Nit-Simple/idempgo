package idempgo

import (
	"time"
)

type RecordStatus int
type AcquiredResult int

const (
	Acquired AcquiredResult = iota
	Replayed
	Conflict
)
const (
	STATUS_IN_FLIGHT RecordStatus = iota
	STATUS_COMPLETED
)

type IdempotencyRecord struct {
	Key         string
	FingerPrint string
	Status      RecordStatus
	Response    *StoredResponse
	ExpiresAt   time.Time
}

type StoredResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

type ResolvedKey struct {
	Key     string
	Derived bool
}

type AcquireOutcome struct {
	Result AcquiredResult
	Record *IdempotencyRecord
}
