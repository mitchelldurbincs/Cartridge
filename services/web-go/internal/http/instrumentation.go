package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/web/internal/logging"
)

func (h Handler) instrument(name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := newResponseWriter(w)

		next(wrapped, r)

		status := wrapped.Status()
		if status == 0 {
			status = http.StatusOK
		}

		elapsed := time.Since(start)
		code := strconv.Itoa(status)

		requestTotal.WithLabelValues(name, code).Inc()
		requestDuration.WithLabelValues(name, code).Observe(elapsed.Seconds())

		if h.logger != nil && logging.ShouldLog(h.level, zerolog.InfoLevel) {
			h.logger.Info().
				Str("service", h.service).
				Str("handler", name).
				Str("request_id", getRequestID(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Dur("duration", elapsed).
				Msg("request completed")
		}
	}
}
