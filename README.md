# log-sqlite

A small structured logger for Go with a pluggable sink and a built-in SQLite backend.

- Leveled logging (`debug`, `info`, `warn`, `error`) with level filtering
- Structured key/value attributes
- Pluggable `Sink` interface
- SQLite sink using pure-Go `modernc.org/sqlite` (no cgo)
- Optional stderr mirror and automatic pruning
- Bridges the standard library `log` package into the same sink

## Install

```sh
go get github.com/G226Medi03/log-sqlite
```

## Quick start

```go
package main

import (
	"os"

	logsqlite "github.com/G226Medi03/log-sqlite"
)

func main() {
	app, err := logsqlite.Open(logsqlite.OpenFromEnv("server"))
	if err != nil {
		panic(err)
	}
	defer app.Close()

	logsqlite.SetDefault(app.Logger)
	logsqlite.InstallStdlogBridge(os.Stderr)

	logsqlite.Info("server started", logsqlite.AttrOf("port", 8080))
}
```

`logsqlite.Open` creates the database and schema on first use. If the
database cannot be opened, fall back to stderr-only logging:

```go
app, err := logsqlite.Open(logsqlite.OpenFromEnv("server"))
if err != nil {
	logger := logsqlite.NewLogger(nil, logsqlite.Options{Component: "server", AlsoStderr: true})
	logsqlite.SetDefault(logger)
} else {
	defer app.Close()
	logsqlite.SetDefault(app.Logger)
}
```

## Configuration

```go
cfg := logsqlite.Config{
	Options: logsqlite.Options{
		Component:  "worker",
		MinLevel:   logsqlite.LevelInfo,
		AlsoStderr: true,
		Stderr:     os.Stderr,
	},
	Path:      "./db/logs.db",
	PruneDays: 30, // delete entries older than 30 days on open
}
```

`OpenFromEnv` reads the same settings from the environment:

| Variable         | Default     | Description                              |
| ---------------- | ----------- | ---------------------------------------- |
| `LOG_DB`         | `./logs.db` | SQLite database path                     |
| `LOG_LEVEL`      | `info`      | `debug`, `info`, `warn` or `error`       |
| `LOG_STDERR`     | `true`      | Also write entries to stderr             |
| `LOG_PRUNE_DAYS` | `0`         | Delete entries older than N days on open |

## Custom sinks

```go
type Sink interface {
	Insert(entry Entry) error
}

logger := logsqlite.NewLogger(mySink, logsqlite.Options{
	Component:  "worker",
	MinLevel:   logsqlite.LevelDebug,
	AlsoStderr: true,
})

logger.With(logsqlite.AttrOf("request_id", id)).Info("handled")
```

Loggers are safe for concurrent use. Entries carry UTC timestamps, and
attributes are written to stderr in sorted `key=value` order.

## SQLite schema

```sql
CREATE TABLE log_entries (
	log_id    INTEGER PRIMARY KEY AUTOINCREMENT,
	time      DATETIME NOT NULL,
	level     TEXT NOT NULL,
	component TEXT NOT NULL,
	message   TEXT NOT NULL,
	attrs     TEXT NOT NULL DEFAULT ''
);
```

`attrs` holds a JSON object of the entry attributes. Indexes exist on
`time`, `level` and `component` for direct queries with `sqlite3`.

## License

MIT
