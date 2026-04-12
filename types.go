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
	key         string
	fingerPrint string
	status      RecordStatus
	Response    *StoredResponse
	ExpiresAt   time.Time
}

type StoredResponse struct {
	statusCode int
	headers    map[string]string
	body       []byte
}

type ResolvedKey struct {
	key     string
	derived bool
}

type AcquireOutcome struct {
	Result AcquiredResult
	Record *IdempotencyRecord
}
