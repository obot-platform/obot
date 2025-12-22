package handlers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gptscript-ai/go-gptscript"
)

// StreamLogsOptions configures SSE log streaming behavior.
type StreamLogsOptions struct {
	// SendKeepAlive enables periodic keep-alive pings to prevent connection timeout.
	SendKeepAlive bool
	// KeepAliveInterval sets the interval for keep-alive pings (default 30s).
	KeepAliveInterval time.Duration
	// SendDisconnect enables sending a disconnect event when the client disconnects.
	SendDisconnect bool
	// SendEnded enables sending an ended event when the log stream ends.
	SendEnded bool
}

// StreamLogs streams logs from an io.ReadCloser to an HTTP response as Server-Sent Events.
// It handles:
// - SSE header setup (Content-Type, Cache-Control, Connection)
// - Docker log header stripping (8-byte prefix for stdout/stderr)
// - Context cancellation
// - Optional keep-alive pings
// - Proper SSE event formatting
func StreamLogs(ctx context.Context, w http.ResponseWriter, logs io.ReadCloser, opts StreamLogsOptions) error {
	defer logs.Close()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, shouldFlush := w.(http.Flusher)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: Log stream started\n\n")
	if shouldFlush {
		flusher.Flush()
	}

	// Channel to coordinate between goroutines
	logChan := make(chan string, 100) // Buffered to prevent blocking

	// Start a goroutine to read logs
	go func() {
		defer close(logChan)

		scanner := bufio.NewScanner(logs)
		for scanner.Scan() {
			line := stripDockerLogHeader(scanner.Text())
			select {
			case logChan <- line:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			// Send error event
			select {
			case logChan <- fmt.Sprintf("ERROR retrieving logs: %v", err):
			case <-ctx.Done():
			}
		}
	}()

	// Setup optional keep-alive ticker
	var ticker *time.Ticker
	var tickerC <-chan time.Time
	if opts.SendKeepAlive {
		interval := opts.KeepAliveInterval
		if interval == 0 {
			interval = 30 * time.Second
		}
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	// Send log events as they come in
	for {
		select {
		case <-ctx.Done():
			if opts.SendDisconnect {
				fmt.Fprintf(w, "event: disconnected\ndata: Client disconnected\n\n")
				if shouldFlush {
					flusher.Flush()
				}
			}
			return nil
		case <-tickerC:
			// Send keep-alive ping
			fmt.Fprintf(w, "event: ping\ndata: keep-alive\n\n")
			if shouldFlush {
				flusher.Flush()
			}
		case logLine, ok := <-logChan:
			if !ok {
				if opts.SendEnded {
					fmt.Fprintf(w, "event: ended\ndata: Log stream ended\n\n")
					if shouldFlush {
						flusher.Flush()
					}
				}
				return nil
			}
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", logLine)
			if shouldFlush {
				flusher.Flush()
			}
		}
	}
}

// stripDockerLogHeader removes the 8-byte Docker log header from a line if present.
// Docker prepends a header to each log line containing stream type (stdout/stderr) and length.
// See https://github.com/moby/moby/issues/7375#issuecomment-51462963
func stripDockerLogHeader(line string) string {
	if len(line) > 0 && (line[0] == '\x01' || line[0] == '\x02') {
		if len(line) > 8 {
			return line[8:]
		}
		return ""
	}
	return line
}

// DeleteCredentialIfExists removes a credential if it exists.
// Does not return an error if the credential is not found.
func DeleteCredentialIfExists(ctx context.Context, gptClient *gptscript.GPTScript, credCtxs []string, toolName string) error {
	cred, err := gptClient.RevealCredential(ctx, credCtxs, toolName)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil // Credential doesn't exist, nothing to delete
		}
		return fmt.Errorf("failed to find credential: %w", err)
	}

	if err = gptClient.DeleteCredential(ctx, cred.Context, toolName); err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}
	return nil
}

// EnsureCredential will ensure a given credential exists and it's env field is up to date.
// Returns true if a credential was created or modified to match.
func ensureCredential(ctx context.Context, gptClient *gptscript.GPTScript, cred gptscript.Credential) (bool, error) {
	credCtx := []string{cred.Context}
	existing, err := gptClient.RevealCredential(ctx, credCtx, cred.ToolName)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return false, fmt.Errorf("failed to find credential: %w", err)
		}

		// Create new credential
		if err := gptClient.CreateCredential(ctx, cred); err != nil {
			return false, fmt.Errorf("failed to create credential: %w", err)
		}
		return true, nil
	}

	// Existing credential, check if it needs an update
	var modified bool
	for key, env := range cred.Env {
		existingEnv, ok := existing.Env[key]
		if !ok || existingEnv != env {
			modified = true
			break
		}
	}
	if !modified && len(cred.Env) == len(existing.Env) {
		// No update needed
		return false, nil
	}

	if err := gptClient.CreateCredential(ctx, cred); err != nil {
		return false, fmt.Errorf("failed to create credential: %w", err)
	}

	return true, nil
}
