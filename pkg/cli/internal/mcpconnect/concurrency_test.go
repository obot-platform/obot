package mcpconnect

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// The full OAuth exchange is covered in auth_test.go. This test handler records
// login attempts across processes and persists their resulting credentials.
type recordingLogin struct {
	auth.OAuthHandler
	store    *tokenStore
	endpoint string
}

func (h *recordingLogin) Authorize(ctx context.Context, _ *http.Request, _ *http.Response) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login: HTTP %d", resp.StatusCode)
	}
	return h.store.save(&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "shared-access", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)})
}

func TestCredentialProcess(t *testing.T) {
	mode := os.Getenv("OBOT_CREDENTIAL_TEST_MODE")
	if mode == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	store := newTokenStore(os.Getenv("OBOT_CREDENTIAL_TEST_DIR"), "https://obot.example/mcp-connect/shared")
	fmt.Println("ready")
	switch mode {
	case "hold":
		unlock, err := store.lock(ctx)
		require.NoError(t, err)
		defer unlock()
		fmt.Println("locked")
		<-ctx.Done()
	case "refresh":
		source, err := store.load(ctx, http.DefaultClient)
		require.NoError(t, err)
		require.NotNil(t, source)
		token, err := source.Token()
		require.NoError(t, err)
		require.Equal(t, "shared-access", token.AccessToken)
	case "login":
		handler := &serializedOAuthHandler{
			OAuthHandler: &recordingLogin{store: store, endpoint: os.Getenv("OBOT_CREDENTIAL_TEST_ENDPOINT")},
			store:        store,
			client:       http.DefaultClient,
		}
		req := httptest.NewRequest(http.MethodGet, "https://obot.example/mcp-connect/shared", nil)
		resp := &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}
		require.NoError(t, handler.Authorize(ctx, req, resp))
		source, err := handler.TokenSource(ctx)
		require.NoError(t, err)
		token, err := source.Token()
		require.NoError(t, err)
		require.Equal(t, "shared-access", token.AccessToken)
	default:
		t.Fatal("unknown subprocess mode")
	}
}

func credentialProcess(t *testing.T, mode, dir, endpoint string) (*exec.Cmd, *bufio.Scanner) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCredentialProcess$")
	cmd.Env = append(os.Environ(), "OBOT_CREDENTIAL_TEST_MODE="+mode, "OBOT_CREDENTIAL_TEST_DIR="+dir, "OBOT_CREDENTIAL_TEST_ENDPOINT="+endpoint)
	pipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	scanner := bufio.NewScanner(pipe)
	require.True(t, scanner.Scan())
	require.Equal(t, "ready", scanner.Text())
	return cmd, scanner
}

func TestConcurrentProcessesShareLoginAndRefresh(t *testing.T) {
	for _, mode := range []string{"login", "refresh"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) != 1 {
					http.Error(w, "credential already consumed", http.StatusBadRequest)
					return
				}
				if mode == "refresh" {
					require.NoError(t, r.ParseForm())
					require.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"access_token":"shared-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			store := newTokenStore(dir, "https://obot.example/mcp-connect/shared")
			if mode == "refresh" {
				require.NoError(t, store.save(&oauth2.Config{
					ClientID: "client",
					Endpoint: oauth2.Endpoint{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams},
				}, &oauth2.Token{AccessToken: "expired", RefreshToken: "old-refresh", Expiry: time.Now().Add(-time.Hour)}))
			}
			// Keep a source alive across the other processes' token rotation.
			previous, err := store.load(t.Context(), server.Client())
			require.NoError(t, err)
			// Hold the gate until both independent processes are started.
			unlock, err := store.lock(t.Context())
			require.NoError(t, err)
			defer unlock()
			first, firstOut := credentialProcess(t, mode, dir, server.URL)
			second, secondOut := credentialProcess(t, mode, dir, server.URL)
			unlock()
			for _, child := range []struct {
				cmd    *exec.Cmd
				output *bufio.Scanner
			}{{cmd: first, output: firstOut}, {cmd: second, output: secondOut}} {
				for child.output.Scan() {
					t.Log(child.output.Text())
				}
				require.NoError(t, child.cmd.Wait())
			}
			if previous != nil {
				token, err := previous.Token()
				require.NoError(t, err)
				require.Equal(t, "rotated-refresh", token.RefreshToken)
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestCredentialLockCancellationAndProcessExit(t *testing.T) {
	dir := t.TempDir()
	holder, output := credentialProcess(t, "hold", dir, "")
	require.True(t, output.Scan())
	require.Equal(t, "locked", output.Text())
	store := newTokenStore(dir, "https://obot.example/mcp-connect/shared")
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := store.lock(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// A different endpoint must not wait on this connection's browser flow.
	other := newTokenStore(dir, "https://obot.example/mcp-connect/other")
	unlock, err := other.lock(t.Context())
	require.NoError(t, err)
	unlock()
	require.NoError(t, holder.Process.Kill())
	require.Error(t, holder.Wait())
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	unlock, err = store.lock(ctx)
	require.NoError(t, err)
	unlock()
}
