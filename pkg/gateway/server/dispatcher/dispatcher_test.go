package dispatcher

import (
	"testing"

	"github.com/obot-platform/obot/logger"
	"github.com/sirupsen/logrus"
)

func TestModelProviderLogLevelEnv(t *testing.T) {
	originalLevel := logrus.GetLevel()
	t.Cleanup(func() {
		logrus.SetLevel(originalLevel)
	})

	t.Run("defaults to info", func(t *testing.T) {
		logrus.SetLevel(logrus.InfoLevel)

		if got := modelProviderLogLevel(); got != "INFO" {
			t.Fatalf("modelProviderLogLevel() = %q, want INFO", got)
		}
	})

	t.Run("uses debug when logger is debug", func(t *testing.T) {
		logger.SetDebug()

		if got := modelProviderLogLevel(); got != "DEBUG" {
			t.Fatalf("modelProviderLogLevel() = %q, want DEBUG", got)
		}
	})
}
