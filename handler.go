package idempgo

import (
	"bytes"
	"net/http"
)

func (m *Middleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			rk, err := m.resolveKey(r.Context(), r)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			fp, err := m.cfg.GenerateFingerprint(r.Context(), r)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			outcome, err := m.AcquireOrGet(r.Context(), m.cfg.InFlightTTL, rk.Key, fp)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			switch outcome.Result {
			case Acquired:
				rec := newResponseRecorder(w)
				next.ServeHTTP(rec, r)
				if rec.statusCode >= 500 {
					m.store.Abandon(r.Context(), rk.Key)
					return
				}
				ttl := m.cfg.ExplicitKeyTTL
				rec.flush()
				m.store.Complete(r.Context(), rk.Key, rec.storedResponse(), ttl)
			case Replayed:
				if fp != outcome.Record.FingerPrint {
					w.WriteHeader(http.StatusUnprocessableEntity)
					w.Write([]byte("idempotency key reused with different request"))
					return
				}
				writeReplay(w, outcome.Record)
			case Conflict:
				switch m.cfg.ConflictPolicy {
				case ConflictReject:
					w.WriteHeader(http.StatusConflict)
					w.Write([]byte("the key is already in use"))
					return
				case ConflictBlock:
					outcome, err := m.blockUntillResolved(r.Context(), rk.Key, m.cfg.InFlightTTL, fp)
					if err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					if outcome.Result == Acquired {
						rec := newResponseRecorder(w)
						next.ServeHTTP(rec, r)
						if rec.statusCode >= 500 {
							m.store.Abandon(r.Context(), rk.Key)
							return
						}
						ttl := m.cfg.ExplicitKeyTTL
						rec.flush()
						m.store.Complete(r.Context(), rk.Key, rec.storedResponse(), ttl)
						return
					}
					if outcome.Result == Replayed {
						if fp != outcome.Record.FingerPrint {
							w.WriteHeader(http.StatusUnprocessableEntity)
							w.Write([]byte("idempotency key reused with different request"))
							return
						}
						writeReplay(w, outcome.Record)
						return
					}
				}
				w.WriteHeader(http.StatusConflict)
				w.Write([]byte("the key is already in use"))
				return // Add this return statement
			}
		})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
	headers    http.Header
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		headers:        make(http.Header),
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.buf.Write(b)
}

func (r *responseRecorder) flush() {
	for k, v := range r.headers {
		for _, vv := range v {
			r.ResponseWriter.Header().Add(k, vv)
		}
	}
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	r.ResponseWriter.WriteHeader(r.statusCode)
	r.ResponseWriter.Write(r.buf.Bytes())
}

func (r *responseRecorder) storedResponse() *StoredResponse {
	headers := make(map[string]string, len(r.headers))
	for k, v := range r.headers {
		headers[k] = v[0] // store first value per header
	}
	return &StoredResponse{
		StatusCode: r.statusCode,
		Body:       r.buf.Bytes(),
		Headers:    headers,
	}
}
func writeReplay(w http.ResponseWriter, record *IdempotencyRecord) {
	for k, v := range record.Response.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Idempotent-Replayed", "true")
	w.WriteHeader(record.Response.StatusCode)
	w.Write(record.Response.Body)
}
