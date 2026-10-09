package clauderunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchOrchestrator(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      func() any
		wantStall bool
	}{
		{
			name: "successful polls continue without hints",
			body: func() any { return map[string]any{"connected": true, "last_poll_at": time.Now().UTC()} },
		},
		{
			name:      "HTTP success but frozen polling",
			body:      func() any { return map[string]any{"connected": true, "last_poll_at": "2026-01-01T00:00:00Z"} },
			wantStall: true,
		},
		{
			name:      "failed polls keep advancing",
			body:      func() any { return map[string]any{"connected": false, "last_poll_at": time.Now().UTC()} },
			wantStall: true,
		},
		{
			name:      "missing health fields",
			body:      func() any { return map[string]any{"status": "ok"} },
			wantStall: true,
		},
		{
			name:      "future timestamp",
			body:      func() any { return map[string]any{"connected": true, "last_poll_at": time.Now().Add(time.Hour)} },
			wantStall: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.body())
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()

			err := watchOrchestrator(ctx, server.URL, 10*time.Millisecond, 200*time.Millisecond)
			if tc.wantStall {
				require.ErrorContains(t, err, "no successful polling progress")
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
		})
	}
}

func TestOrchestratorHealthFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "HTTP error",
			status: http.StatusServiceUnavailable,
			body:   `{}`,
		},
		{
			name:   "malformed JSON",
			status: http.StatusOK,
			body:   `{`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := orchestratorLastPoll(t.Context(), server.Client(), server.URL)
			require.Error(t, err)
		})
	}
}

func TestOrchestratorProcessPortsAndShutdown(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(home, "fake-claude")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nfor arg do port=$arg; done\nprintf '%s' \"$port\" > port\nexec sleep 30\n"), 0700))
	ports := map[int]bool{}
	for range 2 {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			runOrchestratorProcess(ctx, binary, "key", dir, "test-env", os.Environ())
		}()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(7 * time.Second):
				t.Error("orchestrator did not stop")
			}
		})
		var value []byte
		require.Eventually(t, func() bool {
			value, _ = os.ReadFile(filepath.Join(dir, "port"))
			return len(value) > 0
		}, 5*time.Second, 10*time.Millisecond)
		port, err := strconv.Atoi(strings.TrimSpace(string(value)))
		require.NoError(t, err)
		require.NotEqual(t, 8080, port)
		require.False(t, ports[port], "environments must have separate health ports")
		ports[port] = true
	}
}

func TestWatchOrchestratorRecoversFromTransientFailures(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": true, "last_poll_at": time.Now().UTC()})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, watchOrchestrator(ctx, server.URL, 10*time.Millisecond, 200*time.Millisecond), context.DeadlineExceeded)
	require.Greater(t, requests.Load(), int32(3))
}

func TestWatchOrchestratorUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.ErrorContains(t, watchOrchestrator(ctx, server.URL, 10*time.Millisecond, 100*time.Millisecond), "no successful polling progress")
}
