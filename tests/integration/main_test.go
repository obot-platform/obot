//go:build integration

package integration

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/obot-platform/obot/tests/integration/harness"
)

func TestMain(m *testing.M) {
	app, err := startObotApplication()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start Obot integration application: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	if err := app.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop Obot integration application: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func startObotApplication() (*harness.App, error) {
	app, err := harness.StartApp(nil)
	if err != nil {
		return nil, err
	}
	if err := app.WaitForDefaultCatalog(2 * time.Minute); err != nil {
		return nil, errors.Join(err, app.Stop())
	}
	return app, nil
}
