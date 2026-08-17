package dispatcher

import (
	"log/slog"
	"testing"

	"github.com/obot-platform/obot/logger"
)

func TestProviderLogLevelEnv(t *testing.T) {
	originalLevel := logger.Level()
	t.Cleanup(func() {
		logger.SetLevel(originalLevel)
	})

	t.Run("defaults to info", func(t *testing.T) {
		logger.SetLevel(slog.LevelInfo)

		if got := providerLogLevel(); got != "INFO" {
			t.Fatalf("providerLogLevel() = %q, want INFO", got)
		}
	})

	t.Run("uses debug when logger is debug", func(t *testing.T) {
		logger.SetDebug()

		if got := providerLogLevel(); got != "DEBUG" {
			t.Fatalf("providerLogLevel() = %q, want DEBUG", got)
		}
	})
}
