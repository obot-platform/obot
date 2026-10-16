// Package modeldatatest provides model data for tests: a few real entries, converted the way
// Obot's model data is, so tests that need model prices don't depend on the data a build
// downloads.
package modeldatatest

import (
	"embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/pkg/modeldata"
)

var (
	//go:embed testdata
	testdata embed.FS
)

// Write writes the test model data into the gateway data dir dataDir, where the gateway
// keeps it in place of data a build bundled. It has gpt-4o, gpt-4o-mini, claude-sonnet-4-5
// and claude-haiku-4-5.
func Write(tb testing.TB, dataDir string) {
	tb.Helper()
	pricingPath, paramsPath := modeldata.Paths(dataDir)
	for path, name := range map[string]string{pricingPath: "testdata/pricing.json", paramsPath: "testdata/model-parameters.json"} {
		data, err := testdata.ReadFile(name)
		if err != nil {
			tb.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			tb.Fatal(err)
		}
	}
}
