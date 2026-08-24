package api

import (
	"log"
	"net/http"
	"time"
)

// Logging returns a middleware that logs each request's method, path, status
// and duration. It wraps the response writer to capture the status code.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// Recover returns a middleware that recovers a panicking handler, logs the
// panic, and writes a 500 response instead of crashing the server.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, err)
				writeError(w, http.StatusInternalServerError, "store_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID returns a middleware that stamps every response with an
// X-Request-ID header derived from the request or a fresh value.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// newRequestID returns a short pseudo-unique request id without external
// dependencies (monotonic counter + timestamp).
func newRequestID() string {
	return "req-" + time.Now().Format("150405.000000000")
}
