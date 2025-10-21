package logging

import (
	"strings"

	"github.com/rs/zerolog"
)

// ParseLevel translates a string value into a zerolog level.
func ParseLevel(value string) zerolog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// ShouldLog reports whether a message at the provided level should be emitted.
func ShouldLog(min, level zerolog.Level) bool {
	return level >= min
}
