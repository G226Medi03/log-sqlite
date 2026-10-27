package logsqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultPath is the log database path used when Config.Path is empty.
const DefaultPath = "./logs.db"

// Config configures Open.
type Config struct {
	Options

	// Path is the SQLite database file. Empty means DefaultPath.
	Path string

	// PruneDays deletes entries older than this many days on open.
	// Zero disables pruning.
	PruneDays int

	// PruneNow overrides the current time used for pruning. Zero means now.
	PruneNow time.Time
}

// DB is a SQLite-backed Sink.
type DB struct {
	ctx context.Context
	db  *sql.DB
}

// OpenSQLite opens the SQLite log database at path, creating the schema if needed.
func OpenSQLite(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open log database: %w", err)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode = WAL;").Scan(&mode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set log database journal mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set log database busy timeout: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &DB{ctx: context.Background(), db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (d *DB) init() error {
	const schema = `CREATE TABLE IF NOT EXISTS log_entries (
		log_id INTEGER PRIMARY KEY AUTOINCREMENT,
		time DATETIME NOT NULL,
		level TEXT NOT NULL,
		component TEXT NOT NULL,
		message TEXT NOT NULL,
		attrs TEXT NOT NULL DEFAULT ''
	);`
	if _, err := d.db.ExecContext(d.ctx, schema); err != nil {
		return fmt.Errorf("create log_entries table: %w", err)
	}

	for _, index := range []string{
		"CREATE INDEX IF NOT EXISTS idx_log_entries_time ON log_entries(time);",
		"CREATE INDEX IF NOT EXISTS idx_log_entries_level ON log_entries(level);",
		"CREATE INDEX IF NOT EXISTS idx_log_entries_component ON log_entries(component);",
	} {
		if _, err := d.db.ExecContext(d.ctx, index); err != nil {
			return fmt.Errorf("create log_entries index: %w", err)
		}
	}
	return nil
}

// Insert implements Sink.
func (d *DB) Insert(entry Entry) error {
	attrs, err := encodeAttrs(entry.Attrs)
	if err != nil {
		return err
	}

	_, err = d.db.ExecContext(d.ctx,
		`INSERT INTO log_entries (time, level, component, message, attrs) VALUES (?, ?, ?, ?, ?)`,
		entry.Time, entry.Level.String(), entry.Component, entry.Message, attrs)
	if err != nil {
		return fmt.Errorf("insert log entry: %w", err)
	}
	return nil
}

// Prune deletes entries older than before.
func (d *DB) Prune(before time.Time) (int64, error) {
	result, err := d.db.ExecContext(d.ctx, `DELETE FROM log_entries WHERE time < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("prune log entries: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned log entries: %w", err)
	}
	return affected, nil
}

// Count returns the number of stored entries.
func (d *DB) Count() (int, error) {
	var count int
	if err := d.db.QueryRowContext(d.ctx, `SELECT COUNT(*) FROM log_entries`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count log entries: %w", err)
	}
	return count, nil
}

// Close closes the underlying database.
func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func encodeAttrs(attrs map[string]string) (string, error) {
	if len(attrs) == 0 {
		return "", nil
	}
	data, err := json.Marshal(attrs)
	if err != nil {
		return "", fmt.Errorf("encode log attrs: %w", err)
	}
	return string(data), nil
}

// App is a Logger backed by a SQLite database.
type App struct {
	*Logger

	sink *DB
}

// Open opens the log database described by cfg and returns a ready-to-use App.
func Open(cfg Config) (*App, error) {
	sink, err := OpenSQLite(cfg.Path)
	if err != nil {
		return nil, err
	}

	if cfg.PruneDays > 0 {
		now := cfg.PruneNow
		if now.IsZero() {
			now = time.Now().UTC()
		}
		_, _ = sink.Prune(now.AddDate(0, 0, -cfg.PruneDays))
	}

	logger := NewLogger(sink, cfg.Options)
	return &App{Logger: logger, sink: sink}, nil
}

// Prune deletes entries older than before.
func (a *App) Prune(before time.Time) (int64, error) {
	if a == nil || a.sink == nil {
		return 0, nil
	}
	return a.sink.Prune(before)
}

// Close closes the log database.
func (a *App) Close() error {
	if a == nil || a.sink == nil {
		return nil
	}
	return a.sink.Close()
}

// OpenFromEnv builds a Config from the environment:
//
//	LOG_DB          database path (default DefaultPath)
//	LOG_LEVEL       debug, info, warn or error (default info)
//	LOG_STDERR      mirror entries to stderr (default true)
//	LOG_PRUNE_DAYS  delete entries older than N days on open (default 0)
func OpenFromEnv(component string) Config {
	return Config{
		Options: Options{
			Component:  component,
			MinLevel:   ParseLevel(os.Getenv("LOG_LEVEL"), LevelInfo),
			AlsoStderr: envBool("LOG_STDERR", true),
			Stderr:     os.Stderr,
		},
		Path:      envOr("LOG_DB", DefaultPath),
		PruneDays: envInt("LOG_PRUNE_DAYS", 0),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
