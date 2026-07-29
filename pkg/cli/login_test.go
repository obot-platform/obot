package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient"
	"github.com/spf13/cobra"
)

func TestLoginOutput(t *testing.T) {
	tests := []struct {
		name       string
		printToken bool
		wantStdout string
	}{
		{
			name: "normal login keeps stdout empty",
		},
		{
			name:       "print token writes only token to stdout",
			printToken: true,
			wantStdout: "ok1-test-token\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := (&apiclient.Client{BaseURL: "https://obot.example.com/api"}).WithTokenFetcher(
				func(context.Context, string, apiclient.TokenFetchOptions) (string, error) {
					fmt.Fprintln(os.Stderr, "Open https://obot.example.com/oauth2/start and enter code 7KDM-PQ4W to authenticate.")
					return "ok1-test-token", nil
				},
			)
			login := &Login{
				PrintToken: tt.printToken,
				root:       &Obot{Client: client},
			}
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())

			stdout, stderr := captureProcessOutput(t, func() {
				if err := login.Run(cmd, nil); err != nil {
					t.Fatal(err)
				}
			})

			if stdout != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr, "Open https://obot.example.com/oauth2/start and enter code 7KDM-PQ4W") {
				t.Fatalf("expected device guidance on stderr, got %q", stderr)
			}
			if !strings.Contains(stderr, "Logged in to https://obot.example.com/api") {
				t.Fatalf("expected login status on stderr, got %q", stderr)
			}
		})
	}
}

func captureProcessOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	defer func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
	}()

	fn()

	if err := stdoutWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatal(err)
	}
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	if err := stdoutReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrReader.Close(); err != nil {
		t.Fatal(err)
	}

	return string(stdout), string(stderr)
}
