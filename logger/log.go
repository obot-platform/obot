// Package logger installs the process-wide slog default so that the application
// produces structured logs. Log with log/slog directly; nothing here belongs on
// the call-site path.
package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	// Text is human-readable output, for interactive use.
	Text Format = "text"
	// JSON is the encoding log collectors parse. Long-running services use it.
	JSON Format = "json"
)

var (
	level = new(slog.LevelVar)

	// root is trimmed off source paths so entries carry a repo-relative location
	// rather than whatever absolute path the build happened to use. This file is
	// <root>/logger/log.go, so its own compiled-in path locates the repo.
	root = func() string {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return ""
		}
		return filepath.Dir(filepath.Dir(file)) + string(filepath.Separator)
	}()
)

// Format selects the output encoding. The zero value is Text.
type Format string

// Setup installs the process-wide slog default in the given format.
//
// It must be called at run time rather than from an init function.
// slog.SetDefault is process-global, and dependencies install their own
// handlers from their init functions - nanobot does, for one. Package
// initialization order decides who wins, so an init here would be a coin flip.
// main runs after every init, so a call from there always takes effect.
func Setup(format Format) {
	options := slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch format {
	case JSON:
		// Only the collector cares about source locations and the renamed
		// keys; both are noise in a terminal.
		options.AddSource = true
		options.ReplaceAttr = replace
		handler = slog.NewJSONHandler(os.Stderr, &options)
	default:
		handler = slog.NewTextHandler(os.Stderr, &options)
	}

	slog.SetDefault(slog.New(handler))
}

// SetDebug enables debug logging.
func SetDebug() {
	level.Set(slog.LevelDebug)
}

// SetLevel sets the log level.
func SetLevel(l slog.Level) {
	level.Set(l)
}

// Level reports the current log level.
func Level() slog.Level {
	return level.Level()
}

// IsDebug reports whether debug logging is enabled.
func IsDebug() bool {
	return level.Level() <= slog.LevelDebug
}

// replace renames slog's built-in keys to the ones log collectors recognize.
func replace(groups []string, a slog.Attr) slog.Attr {
	// Only the top-level built-ins are special; leave nested groups alone.
	if len(groups) > 0 {
		return a
	}

	switch a.Key {
	case slog.MessageKey:
		a.Key = "message"
	case slog.LevelKey:
		// The built-in level always carries an slog.Level; an attribute that
		// merely shares the "level" key carries anything. Leave those alone
		// rather than panicking on the assertion.
		level, ok := a.Value.Any().(slog.Level)
		if !ok {
			return a
		}
		a.Key = "severity"
		// Bucket to the names Cloud Logging accepts. Levels between the four
		// standard ones occur in practice: klog verbosity arrives through
		// logr as intermediate levels that slog would render as "DEBUG+2".
		switch {
		case level < slog.LevelInfo:
			a.Value = slog.StringValue("DEBUG")
		case level < slog.LevelWarn:
			a.Value = slog.StringValue("INFO")
		case level < slog.LevelError:
			a.Value = slog.StringValue("WARNING")
		default:
			a.Value = slog.StringValue("ERROR")
		}
	case slog.SourceKey:
		// Flatten to file:line. The function name is both redundant with the
		// location and long enough to bury the message. The key follows
		// Cloud Logging's naming and stays clear of attributes named
		// "source", which several call sites use for domain values.
		if source, ok := a.Value.Any().(*slog.Source); ok {
			a.Key = "sourceLocation"
			a.Value = slog.StringValue(trimSourcePath(source.File) + ":" + strconv.Itoa(source.Line))
		}
	}

	return a
}

// trimSourcePath cuts a compiled-in file path down to a stable, readable
// form: repo-relative for this repository's files, module-path-relative for
// dependencies, matching what building with -trimpath would have recorded.
func trimSourcePath(file string) string {
	if strings.HasPrefix(file, root) {
		return file[len(root):]
	}

	// Dependencies come from the module cache, which by convention lives
	// under a path ending in /pkg/mod/. What follows is the module path and
	// version: k8s.io/apiserver@v0.36.2/pkg/....
	if _, after, ok := strings.Cut(file, "/pkg/mod/"); ok {
		return after
	}

	return file
}
