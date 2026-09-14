// Package logging sets up the process-wide structured logger.
//
// All log lines go through slog so every field named in the PRD's error
// handling rules (gitlab_project_id, gitlab_issue_iid, event_kind, ...) is
// grep-able key=value output, not free-form prose.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a slog.Logger from the config file's logging.level and
// logging.format values ("debug|info|warn|error" and "text|json").
// Unknown values fall back to info/text rather than failing startup over a
// logging typo.
func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
