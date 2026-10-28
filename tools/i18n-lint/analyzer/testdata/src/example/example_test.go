package example

import (
	tr "github.com/obot-platform/obot/pkg/i18n"
	"testing"
)

func TestUsage(t *testing.T) {
	tr.Text("en", "missing") // want `message "missing" is missing or empty in ja`
}
