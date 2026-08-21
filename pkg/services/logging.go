package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/go-logr/logr"
	nahlog "github.com/obot-platform/nah/pkg/log"
	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// slogAdapter satisfies nah's printf-style log.Logger interface.
type slogAdapter struct{}

// slogHook re-emits every logrus entry through slog.
type slogHook struct{}

// SetupLogBridges routes the loggers dependencies use into the current slog
// default. It must run after the slog handler is installed - logr captures the
// handler here, so an earlier call pins the wrong one.
//
// The klog and controller-runtime wiring is permanent; the Kubernetes
// libraries always log through klog. The nah and logrus pieces are
// interception, needed only until nah and kinm migrate to slog themselves.
func SetupLogBridges() {
	l := logr.FromSlogHandler(slog.Default().Handler())
	klog.SetLogger(l)
	crlog.SetLogger(l)

	// nah's package-level defaults are no-ops for Infof and Debugf and raw
	// stdlib text for the rest.
	nahlog.SetLogger(slogAdapter{})

	// nah and kinm log through logrus.StandardLogger, as does kinm's gorm
	// logger. Silence the direct output and re-emit through slog, letting all
	// entries through so slog applies the level filtering. ReportCaller makes
	// logrus record the real call site for the hook to pass along.
	logrus.SetOutput(io.Discard)
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetReportCaller(true)
	logrus.AddHook(slogHook{})
}

func (slogAdapter) Infof(msg string, args ...any)  { logf(slog.LevelInfo, msg, args...) }
func (slogAdapter) Warnf(msg string, args ...any)  { logf(slog.LevelWarn, msg, args...) }
func (slogAdapter) Errorf(msg string, args ...any) { logf(slog.LevelError, msg, args...) }
func (slogAdapter) Debugf(msg string, args ...any) { logf(slog.LevelDebug, msg, args...) }

func (slogAdapter) Fatalf(msg string, args ...any) {
	logf(slog.LevelError, msg, args...)
	os.Exit(1)
}

func logf(level slog.Level, msg string, args ...any) {
	ctx := context.Background()
	logger := slog.Default()
	if !logger.Enabled(ctx, level) {
		return
	}

	// Attribute the record to the adapter method's caller, not this file.
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip Callers, logf, and the adapter method
	record := slog.NewRecord(time.Now(), level, fmt.Sprintf(msg, args...), pcs[0])
	_ = logger.Handler().Handle(ctx, record)
}

func (slogHook) Levels() []logrus.Level { return logrus.AllLevels }

func (slogHook) Fire(entry *logrus.Entry) error {
	var level slog.Level
	switch entry.Level {
	case logrus.TraceLevel, logrus.DebugLevel:
		level = slog.LevelDebug
	case logrus.InfoLevel:
		level = slog.LevelInfo
	case logrus.WarnLevel:
		level = slog.LevelWarn
	default: // Error, Fatal, Panic
		level = slog.LevelError
	}

	ctx := entry.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger := slog.Default()
	if !logger.Enabled(ctx, level) {
		return nil
	}

	// Attribute the record to the logrus call site, which ReportCaller
	// resolved to the first frame outside logrus.
	var pc uintptr
	if entry.Caller != nil {
		pc = entry.Caller.PC
	}

	record := slog.NewRecord(entry.Time, level, entry.Message, pc)
	for _, key := range slices.Sorted(maps.Keys(entry.Data)) {
		record.AddAttrs(slog.Any(key, entry.Data[key]))
	}
	_ = logger.Handler().Handle(ctx, record)
	return nil
}
