package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
						slog.String("path", r.URL.Path),
					)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal_error"}`))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set("X-Request-ID", id)
			next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
