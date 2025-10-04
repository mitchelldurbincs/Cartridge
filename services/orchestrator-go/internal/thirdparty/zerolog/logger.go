package zerolog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type Logger struct {
	writer     io.Writer
	baseFields map[string]string
}

type Context struct {
	logger *Logger
}

type Event struct {
	logger *Logger
	level  string
	fields map[string]interface{}
	err    error
}

func New(w io.Writer) *Logger {
	if w == nil {
		w = io.Discard
	}
	return &Logger{writer: w, baseFields: map[string]string{}}
}

func (l *Logger) With() Context {
	return Context{logger: l}
}

func (c Context) Timestamp() Context { return c }

func (c Context) Logger() *Logger { return c.logger }

func (l *Logger) log(level string) *Event {
	return &Event{
		logger: l,
		level:  level,
		fields: map[string]interface{}{},
	}
}

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (l *Logger) Info() *Event  { return l.log("info") }
func (l *Logger) Warn() *Event  { return l.log("warn") }
func (l *Logger) Error() *Event { return l.log("error") }
func (l *Logger) Fatal() *Event { return l.log("fatal") }

func (l *Logger) WithLevel(level Level) *Event {
	switch level {
	case InfoLevel:
		return l.Info()
	case WarnLevel:
		return l.Warn()
	case ErrorLevel:
		return l.Error()
	case FatalLevel:
		return l.Fatal()
	default:
		return l.Info()
	}
}

func (e *Event) Str(key, value string) *Event {
	e.fields[key] = value
	return e
}

func (e *Event) Int(key string, value int) *Event {
	e.fields[key] = value
	return e
}

func (e *Event) Int64(key string, value int64) *Event {
	e.fields[key] = value
	return e
}

func (e *Event) Dur(key string, value time.Duration) *Event {
	e.fields[key] = value.String()
	return e
}

func (e *Event) Interface(key string, value interface{}) *Event {
	e.fields[key] = value
	return e
}

func (e *Event) Bytes(key string, value []byte) *Event {
	e.fields[key] = string(value)
	return e
}

func (e *Event) Err(err error) *Event {
	e.err = err
	return e
}

func (e *Event) Msg(msg string) {
	e.fields["msg"] = msg
	if e.err != nil {
		e.fields["error"] = e.err.Error()
	}
	e.logger.output(e.level, e.fields)
	if e.level == "fatal" {
		os.Exit(1)
	}
}

func (l *Logger) output(level string, fields map[string]interface{}) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(l.writer, "%s level=%s", timestamp, level)
	for k, v := range l.baseFields {
		fmt.Fprintf(l.writer, " %s=%s", k, v)
	}
	for k, v := range fields {
		switch val := v.(type) {
		case string:
			fmt.Fprintf(l.writer, " %s=%s", k, val)
		case int, int64, float64:
			fmt.Fprintf(l.writer, " %s=%v", k, val)
		default:
			if jsonVal, err := json.Marshal(val); err == nil {
				fmt.Fprintf(l.writer, " %s=%s", k, string(jsonVal))
			} else {
				fmt.Fprintf(l.writer, " %s=%v", k, val)
			}
		}
	}
	fmt.Fprintln(l.writer)
}
