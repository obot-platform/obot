package dispatcher

import (
	"reflect"
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

		want := map[string]string{"LOG_LEVEL": "INFO"}
		if got := modelProviderLogLevelEnv(); !reflect.DeepEqual(got, want) {
			t.Fatalf("modelProviderLogLevelEnv() = %v, want %v", got, want)
		}
	})

	t.Run("uses debug when logger is debug", func(t *testing.T) {
		logger.SetDebug()

		want := map[string]string{"LOG_LEVEL": "DEBUG"}
		if got := modelProviderLogLevelEnv(); !reflect.DeepEqual(got, want) {
			t.Fatalf("modelProviderLogLevelEnv() = %v, want %v", got, want)
		}
	})
}
