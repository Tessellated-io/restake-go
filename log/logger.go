package log

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// This package implements a hierarchical logger which allows adding prefixes.

type Logger struct {
	// Internal logger
	zerolog.Logger

	// The current prefix
	prefix string
}

func NewLogger() *Logger {
	prefix := ""
	return newLoggerWithPrefix(prefix)
}

func newLoggerWithPrefix(prefix string) *Logger {
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("%s%s", i, prefix))
	}
	log := zerolog.New(output).With().Timestamp().Logger()

	return &Logger{
		prefix: prefix,
		Logger: log,
	}
}

func (l *Logger) ApplyPrefix(additionalPrefix string) *Logger {
	newPrefix := fmt.Sprintf("%s%s", l.prefix, additionalPrefix)

	return newLoggerWithPrefix(newPrefix)
}
