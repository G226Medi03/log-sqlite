// Package logsqlite is a small structured logger with a pluggable sink.
//
// Loggers write entries to a [Sink]. Use [Open] for the built-in SQLite sink,
// or [NewLogger] with a custom sink.
package logsqlite

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level is the severity of a log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the upper-case name of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	}
	return "INFO"
}

// ParseLevel converts text to a Level, returning fallback for unknown values.
func ParseLevel(text string, fallback Level) Level {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	}
	return fallback
}

// Attr is a key/value pair attached to a log entry.
type Attr struct {
	Key   string
	Value string
}

// AttrOf builds an Attr from any value.
func AttrOf(key string, value any) Attr {
	return Attr{Key: key, Value: fmt.Sprintf("%v", value)}
}

// Entry is a single log record handed to a Sink.
type Entry struct {
	Time      time.Time
	Level     Level
	Component string
	Message   string
	Attrs     map[string]string
}

// Sink stores log entries. Implementations must be safe for concurrent use.
type Sink interface {
	Insert(entry Entry) error
}

// Options configures a Logger.
type Options struct {
	Component  string
	MinLevel   Level
	AlsoStderr bool
	Stderr     io.Writer
}

// Logger writes entries to a Sink and optionally mirrors them to stderr.
type Logger struct {
	sink       Sink
	component  string
	minLevel   Level
	alsoStderr bool
	stderr     io.Writer
	baseAttrs  map[string]string
}

// NewLogger returns a Logger that writes to sink.
// A nil sink is allowed and only produces stderr output.
func NewLogger(sink Sink, opts Options) *Logger {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	return &Logger{
		sink:       sink,
		component:  opts.Component,
		minLevel:   opts.MinLevel,
		alsoStderr: opts.AlsoStderr,
		stderr:     stderr,
	}
}

// With returns a copy of the logger with extra attributes.
func (l *Logger) With(attrs ...Attr) *Logger {
	if l == nil {
		return nil
	}
	clone := *l
	clone.baseAttrs = mergeAttrs(l.baseAttrs, attrs)
	return &clone
}

// Enabled reports whether the logger emits entries at level.
func (l *Logger) Enabled(level Level) bool {
	return l != nil && level >= l.minLevel
}

func (l *Logger) Debug(msg string, attrs ...Attr) { l.write(LevelDebug, msg, attrs...) }
func (l *Logger) Info(msg string, attrs ...Attr)  { l.write(LevelInfo, msg, attrs...) }
func (l *Logger) Warn(msg string, attrs ...Attr)  { l.write(LevelWarn, msg, attrs...) }
func (l *Logger) Error(msg string, attrs ...Attr) { l.write(LevelError, msg, attrs...) }

// Fatal logs at error level and exits the process.
func (l *Logger) Fatal(msg string, attrs ...Attr) {
	l.write(LevelError, msg, attrs...)
	exit(1)
}

func (l *Logger) write(level Level, msg string, attrs ...Attr) {
	if !l.Enabled(level) {
		return
	}

	entry := Entry{
		Time:      time.Now().UTC(),
		Level:     level,
		Component: l.component,
		Message:   msg,
		Attrs:     mergeAttrs(l.baseAttrs, attrs),
	}

	if l.alsoStderr {
		fmt.Fprintf(l.stderr, "%s %-5s [%s] %s%s\n",
			entry.Time.Format(time.RFC3339), entry.Level, entry.Component, entry.Message, formatAttrs(entry.Attrs))
	}
	if l.sink == nil {
		return
	}
	if err := l.sink.Insert(entry); err != nil {
		fmt.Fprintf(l.stderr, "[log-sqlite] failed to store log entry: %v\n", err)
	}
}

func mergeAttrs(base map[string]string, attrs []Attr) map[string]string {
	if len(base) == 0 && len(attrs) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(attrs))
	for k, v := range base {
		merged[k] = v
	}
	for _, a := range attrs {
		merged[a.Key] = a.Value
	}
	return merged
}

func formatAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(" ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(attrs[k])
	}
	return b.String()
}

var exit = os.Exit

var (
	defaultMu     sync.RWMutex
	defaultLogger = &Logger{component: "app", minLevel: LevelInfo, alsoStderr: true, stderr: os.Stderr}
)

// SetDefault replaces the package-level logger used by Debug, Info, Warn, Error and Fatal.
func SetDefault(l *Logger) {
	if l == nil {
		return
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = l
}

// Default returns the current package-level logger.
func Default() *Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// Named returns a logger with the same sink and options as the default logger
// but a different component name.
func Named(component string) *Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()

	base := defaultLogger
	return &Logger{
		sink:       base.sink,
		component:  component,
		minLevel:   base.minLevel,
		alsoStderr: base.alsoStderr,
		stderr:     base.stderr,
	}
}

func Debug(msg string, attrs ...Attr) { Default().Debug(msg, attrs...) }
func Info(msg string, attrs ...Attr)  { Default().Info(msg, attrs...) }
func Warn(msg string, attrs ...Attr)  { Default().Warn(msg, attrs...) }
func Error(msg string, attrs ...Attr) { Default().Error(msg, attrs...) }
func Fatal(msg string, attrs ...Attr) { Default().Fatal(msg, attrs...) }
