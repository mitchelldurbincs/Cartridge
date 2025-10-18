package observability

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Attribute describes contextual metadata for spans.
type Attribute struct {
	Key   string
	Value string
}

// Span represents a tracing span lifecycle.
type Span interface {
	End(err error)
}

// Tracer creates spans.
type Tracer interface {
	Start(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span)
}

type noopTracer struct{}

type noopSpan struct{}

// NoopTracer returns a tracer that drops all spans.
func NoopTracer() Tracer {
	return noopTracer{}
}

func (noopTracer) Start(ctx context.Context, _ string, _ ...Attribute) (context.Context, Span) {
	return ctx, noopSpan{}
}

func (noopSpan) End(error) {}

type loggerTracer struct {
	logger *zerolog.Logger
}

type loggerSpan struct {
	logger *zerolog.Logger
	name   string
	attrs  []Attribute
	start  time.Time
}

// NewLoggerTracer constructs a tracer that logs span boundaries when enabled.
func NewLoggerTracer(logger *zerolog.Logger) Tracer {
	if logger == nil {
		return NoopTracer()
	}
	return &loggerTracer{logger: logger}
}

func (t *loggerTracer) Start(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span) {
	started := time.Now()
	evt := t.logger.WithLevel(zerolog.DebugLevel).Str("span", name).Str("started_at", started.Format(time.RFC3339Nano))
	for _, attr := range attrs {
		evt = evt.Str(attr.Key, attr.Value)
	}
	evt.Msg("span started")
	return ctx, &loggerSpan{logger: t.logger, name: name, attrs: attrs, start: started}
}

func (s *loggerSpan) End(err error) {
	if err != nil {
		evt := s.logger.Error().Str("span", s.name).Dur("duration", time.Since(s.start))
		for _, attr := range s.attrs {
			evt = evt.Str(attr.Key, attr.Value)
		}
		evt.Err(err).Msg("span ended with error")
		return
	}
	evt := s.logger.WithLevel(zerolog.DebugLevel).Str("span", s.name).Dur("duration", time.Since(s.start))
	for _, attr := range s.attrs {
		evt = evt.Str(attr.Key, attr.Value)
	}
	evt.Msg("span ended")
}
