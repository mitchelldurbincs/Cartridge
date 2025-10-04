package zerolog

import (
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
	fields map[string]string
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
		fields: map[string]string{},
	}
}

func (l *Logger) Info() *Event  { return l.log("info") }
func (l *Logger) Warn() *Event  { return l.log("warn") }
func (l *Logger) Error() *Event { return l.log("error") }
func (l *Logger) Fatal() *Event { return l.log("fatal") }

func (e *Event) Str(key, value string) *Event {
	e.fields[key] = value
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

func (l *Logger) output(level string, fields map[string]string) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(l.writer, "%s level=%s", timestamp, level)
	for k, v := range l.baseFields {
		fmt.Fprintf(l.writer, " %s=%s", k, v)
	}
	for k, v := range fields {
		fmt.Fprintf(l.writer, " %s=%s", k, v)
	}
	fmt.Fprintln(l.writer)
}
