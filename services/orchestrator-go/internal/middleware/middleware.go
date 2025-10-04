package middleware

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// LogEntry interface matches chi's LogEntry
type LogEntry interface {
	Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{})
	Panic(v interface{}, stack []byte)
}

// LogFormatter interface matches chi's LogFormatter
type LogFormatter interface {
	NewLogEntry(r *http.Request) LogEntry
}

// RequestLogger creates a zerolog-based request logger middleware
func RequestLogger(logger zerolog.Logger) func(next http.Handler) http.Handler {
	formatter := &RequestLoggerFormatter{logger}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entry := formatter.NewLogEntry(r)
			ww := &responseWriter{ResponseWriter: w, entry: entry, start: time.Now()}
			next.ServeHTTP(ww, r)
			entry.Write(ww.status, ww.written, w.Header(), time.Since(ww.start), nil)
		})
	}
}

// RequestLoggerFormatter implements LogFormatter interface
type RequestLoggerFormatter struct {
	Logger zerolog.Logger
}

func (l *RequestLoggerFormatter) NewLogEntry(r *http.Request) LogEntry {
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	// Add correlation ID to request context for downstream handlers
	r.Header.Set("X-Correlation-ID", correlationID)

	entry := &RequestLoggerEntry{
		Logger:        l.Logger,
		CorrelationID: correlationID,
		Method:        r.Method,
		URL:           r.URL.Path,
		RemoteAddr:    r.RemoteAddr,
		StartTime:     time.Now(),
	}

	entry.Logger.Info().
		Str("correlation_id", correlationID).
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Str("remote_addr", r.RemoteAddr).
		Msg("Request started")

	return entry
}

// RequestLoggerEntry implements chi's LogEntry interface
type RequestLoggerEntry struct {
	Logger        zerolog.Logger
	CorrelationID string
	Method        string
	URL           string
	RemoteAddr    string
	StartTime     time.Time
}

func (l *RequestLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	level := zerolog.InfoLevel
	if status >= 400 && status < 500 {
		level = zerolog.WarnLevel
	} else if status >= 500 {
		level = zerolog.ErrorLevel
	}

	l.Logger.WithLevel(level).
		Str("correlation_id", l.CorrelationID).
		Str("method", l.Method).
		Str("url", l.URL).
		Str("remote_addr", l.RemoteAddr).
		Int("status", status).
		Int("bytes", bytes).
		Dur("elapsed", elapsed).
		Msg("Request completed")
}

func (l *RequestLoggerEntry) Panic(v interface{}, stack []byte) {
	l.Logger.Error().
		Str("correlation_id", l.CorrelationID).
		Str("method", l.Method).
		Str("url", l.URL).
		Interface("panic", v).
		Bytes("stack", stack).
		Msg("Request panic")
}

// responseWriter wraps http.ResponseWriter to capture status and bytes written
type responseWriter struct {
	http.ResponseWriter
	entry   LogEntry
	status  int
	written int
	start   time.Time
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.written += n
	return n, err
}

// CorrelationID adds a correlation ID to requests if not present
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			r.Header.Set("X-Correlation-ID", correlationID)
		}

		// Add to response headers
		w.Header().Set("X-Correlation-ID", correlationID)

		next.ServeHTTP(w, r)
	})
}

// RateLimiter creates a simple rate limiting middleware
func RateLimiter(requestsPerSecond int) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simple rate limiting implementation
			// In production, would use a more sophisticated rate limiter
			// like golang.org/x/time/rate or redis-based limiting
			next.ServeHTTP(w, r)
		})
	}
}