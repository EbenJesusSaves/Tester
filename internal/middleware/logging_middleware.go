package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers and query strings may contain cookies, OAuth codes, signed
		// URLs and device identifiers. A denylist cannot cover new credentials.
		// Log only the escaped path, status and duration; never the panic value.
		path := r.URL.EscapedPath()
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered during %s %s (type %T)\nStack Trace:\n%s",
					r.Method, path, err, debug.Stack())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		if r.URL.Path == "/hc" || r.URL.Path == "/ready" || r.URL.Path == "/metrics" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Telemetry ingestion fires on every app-background of every device;
		// logging each batch would drown the request log.
		if strings.HasSuffix(r.URL.Path, "/v1/logs") || strings.HasSuffix(r.URL.Path, "/v1/metrics") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(recorder, r)

		log.Printf("Completed %s %s %d in %v", r.Method, path, recorder.statusCode, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
