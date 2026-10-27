package logsqlite

import (
	"testing"
	"time"
)

func TestOpenSQLiteInsert(t *testing.T) {
	db, err := OpenSQLite(t.TempDir() + "/logs.db")
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	entry := Entry{
		Time:      time.Now().UTC(),
		Level:     LevelWarn,
		Component: "pipeline",
		Message:   "job failed",
		Attrs:     map[string]string{"file": "a.pdf"},
	}
	if err := db.Insert(entry); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if count, err := db.Count(); err != nil || count != 1 {
		t.Errorf("Count = %d (err=%v), want 1", count, err)
	}
}

func TestDBPrune(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	for _, entry := range []Entry{
		{Time: now.Add(-48 * time.Hour), Level: LevelInfo, Component: "x", Message: "old"},
		{Time: now, Level: LevelInfo, Component: "x", Message: "new"},
	} {
		if err := db.Insert(entry); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	deleted, err := db.Prune(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if count, err := db.Count(); err != nil || count != 1 {
		t.Errorf("Count = %d (err=%v), want 1", count, err)
	}
}

func TestAppOpenWritesThrough(t *testing.T) {
	app, err := Open(Config{
		Options: Options{Component: "server", MinLevel: LevelDebug},
		Path:    ":memory:",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	app.Info("started", AttrOf("port", 8080))

	count, err := app.sink.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestOpenFromEnv(t *testing.T) {
	t.Setenv("LOG_DB", "/tmp/custom.db")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_STDERR", "false")
	t.Setenv("LOG_PRUNE_DAYS", "7")

	cfg := OpenFromEnv("worker")
	if cfg.Path != "/tmp/custom.db" {
		t.Errorf("Path = %q", cfg.Path)
	}
	if cfg.Component != "worker" {
		t.Errorf("Component = %q", cfg.Component)
	}
	if cfg.MinLevel != LevelDebug {
		t.Errorf("MinLevel = %v", cfg.MinLevel)
	}
	if cfg.AlsoStderr {
		t.Error("AlsoStderr = true, want false")
	}
	if cfg.PruneDays != 7 {
		t.Errorf("PruneDays = %d, want 7", cfg.PruneDays)
	}
}

func TestOpenFromEnvDefaults(t *testing.T) {
	for _, key := range []string{"LOG_DB", "LOG_LEVEL", "LOG_STDERR", "LOG_PRUNE_DAYS"} {
		t.Setenv(key, "")
	}

	cfg := OpenFromEnv("worker")
	if cfg.Path != DefaultPath {
		t.Errorf("Path = %q, want %q", cfg.Path, DefaultPath)
	}
	if cfg.MinLevel != LevelInfo {
		t.Errorf("MinLevel = %v, want INFO", cfg.MinLevel)
	}
	if !cfg.AlsoStderr {
		t.Error("AlsoStderr = false, want true")
	}
	if cfg.PruneDays != 0 {
		t.Errorf("PruneDays = %d, want 0", cfg.PruneDays)
	}
}
