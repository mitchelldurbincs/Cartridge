package http

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
)

var idCounter uint64

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddUint64(&idCounter, 1)
		requestID := formatRequestID(id)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func realIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			r.RemoteAddr = ip
		} else if ip := r.Header.Get("X-Real-IP"); ip != "" {
			r.RemoteAddr = ip
		} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			r.RemoteAddr = host
		}
		next.ServeHTTP(w, r)
	})
}

func recovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func timeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "request timed out")
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func formatRequestID(id uint64) string {
	return time.Now().UTC().Format("20060102T150405Z") + "-" + itoa(id)
}

func getRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
