package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBinUsesObotBin(t *testing.T) {
	t.Setenv("OBOT_BIN", "/custom/obot")
	if got := Bin(); got != "/custom/obot" {
		t.Fatalf("Bin() = %q, want %q", got, "/custom/obot")
	}
}

func TestCurrentBinMakesRelativePathAbsolute(t *testing.T) {
	originalArg := os.Args[0]
	os.Args[0] = filepath.Join("relative", "obot")
	t.Cleanup(func() {
		os.Args[0] = originalArg
	})

	want, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := currentBin(); got != want {
		t.Fatalf("currentBin() = %q, want %q", got, want)
	}
}
