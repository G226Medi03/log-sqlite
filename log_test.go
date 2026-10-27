package logsqlite

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

type memorySink struct {
	mu      sync.Mutex
	entries []Entry
}

func (s *memorySink) Insert(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

func (s *memorySink) all() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}

func TestLoggerFiltersByLevel(t *testing.T) {
	sink := &memorySink{}
	logger := NewLogger(sink, Options{Component: "test", MinLevel: LevelWarn})

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	entries := sink.all()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Level != LevelWarn || entries[1].Level != LevelError {
		t.Errorf("levels = %v, %v; want WARN, ERROR", entries[0].Level, entries[1].Level)
	}
}

func TestLoggerWithMergesAttrs(t *testing.T) {
	sink := &memorySink{}
	logger := NewLogger(sink, Options{Component: "svc", MinLevel: LevelDebug}).
		With(AttrOf("request_id", "abc")).
		With(AttrOf("request_id", "xyz"), AttrOf("user", 7))

	logger.Info("hello", AttrOf("duration_ms", 12))

	entries := sink.all()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	attrs := entries[0].Attrs
	if attrs["request_id"] != "xyz" {
		t.Errorf("request_id = %q, want xyz", attrs["request_id"])
	}
	if attrs["user"] != "7" {
		t.Errorf("user = %q, want 7", attrs["user"])
	}
	if attrs["duration_ms"] != "12" {
		t.Errorf("duration_ms = %q, want 12", attrs["duration_ms"])
	}
}

func TestLoggerStderrFormat(t *testing.T) {
	var stderr bytes.Buffer
	logger := NewLogger(nil, Options{Component: "svc", MinLevel: LevelDebug, AlsoStderr: true, Stderr: &stderr})

	logger.Warn("slow request", AttrOf("path", "/api"), AttrOf("ms", 30))

	line := stderr.String()
	for _, want := range []string{"WARN", "[svc]", "slow request", "ms=30 path=/api"} {
		if !strings.Contains(line, want) {
			t.Errorf("stderr %q does not contain %q", line, want)
		}
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	var logger *Logger
	logger.Info("ignored")
	if logger.Enabled(LevelError) {
		t.Error("nil logger should never be enabled")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"debug":   LevelDebug,
		"INFO":    LevelInfo,
		"warning": LevelWarn,
		" error ": LevelError,
		"bogus":   LevelInfo,
	}
	for text, want := range cases {
		if got := ParseLevel(text, LevelInfo); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestStdlogWriterSplitsLines(t *testing.T) {
	sink := &memorySink{}
	writer := &stdlogWriter{logger: NewLogger(sink, Options{Component: "stdlib", MinLevel: LevelDebug})}

	if _, err := writer.Write([]byte("first\nsecond\npart")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := writer.Write([]byte("ial\n\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries := sink.all()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	messages := []string{entries[0].Message, entries[1].Message, entries[2].Message}
	want := []string{"first", "second", "partial"}
	for i := range want {
		if messages[i] != want[i] {
			t.Errorf("messages = %v, want %v", messages, want)
			break
		}
	}
}
