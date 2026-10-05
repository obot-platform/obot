package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWriteFileCommandWritesContentVerbatim(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	if _, err := exec.LookPath("base64"); err != nil {
		t.Skip("base64 not available")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "pwned")
	tests := []struct {
		name     string
		fileName string
		content  string
	}{
		{
			name:     "heredoc delimiter in content",
			fileName: "config",
			content:  "first\nEOF\ntouch " + marker + "\ncat << 'EOF'\nlast",
		},
		{
			name:     "single quotes in content",
			fileName: "quotes",
			content:  "it's '$(touch " + marker + ")'",
		},
		{
			name:     "single quote in file name",
			fileName: "it's",
			content:  "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := filepath.Join(dir, tt.fileName)
			if out, err := exec.Command("sh", "-c", "set -e\n"+writeFileCommand(target, tt.content)).CombinedOutput(); err != nil {
				t.Fatalf("script failed: %v: %s", err, out)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.content+"\n" {
				t.Fatalf("file content = %q, want %q", got, tt.content+"\n")
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("content was executed as a command")
			}
		})
	}
}
