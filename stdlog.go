package logsqlite

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type stdlogWriter struct {
	logger *Logger

	mu        sync.Mutex
	remainder string
}

func (w *stdlogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lines := strings.Split(w.remainder+string(p), "\n")
	w.remainder = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.logger.Info(line)
	}
	return len(p), nil
}

// InstallStdlogBridge forwards output from the standard library logger to
// target and to the default logger's sink under the "stdlib" component.
func InstallStdlogBridge(target io.Writer) *Logger {
	if target == nil {
		target = os.Stderr
	}

	bridge := Named("stdlib")
	bridge.alsoStderr = false
	bridge.stderr = os.Stderr

	log.SetOutput(io.MultiWriter(target, &stdlogWriter{logger: bridge}))
	return bridge
}
