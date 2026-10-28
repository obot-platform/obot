package mcpgateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/mmmcp/component"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type compositeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f compositeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCompositeRequestTransportSelectsRequestCredential(t *testing.T) {
	for _, tt := range []struct {
		name          string
		method        string
		authorization string
		want          string
	}{
		{
			name:          "current frontend call",
			method:        http.MethodPost,
			authorization: "Bearer current-caller",
			want:          "Bearer current-caller",
		},
		{
			name:          "invalid current credential must reach gateway validation",
			method:        http.MethodPost,
			authorization: "Bearer invalid-current-caller",
			want:          "Bearer invalid-current-caller",
		},
		{
			name:          "SSE reconnect retains refreshed credential",
			method:        http.MethodGet,
			authorization: "Bearer expired-opening-credential",
			want:          "Bearer refreshed-connection",
		},
		{
			name:          "session cleanup retains refreshed credential",
			method:        http.MethodDelete,
			authorization: "Bearer expired-opening-credential",
			want:          "Bearer refreshed-connection",
		},
		{
			name:   "no frontend message",
			method: http.MethodPost,
			want:   "Bearer refreshed-connection",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := component.ContextWithRequestHeaders(t.Context(), http.Header{"Authorization": {tt.authorization}})
			req, err := http.NewRequestWithContext(ctx, tt.method, "https://obot.example/mcp-connect/component", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer refreshed-connection")
			var got string
			transport := compositeRequestTransport{base: compositeRoundTripFunc(func(outbound *http.Request) (*http.Response, error) {
				got = outbound.Header.Get("Authorization")
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
			})}
			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, tt.want, got)
			require.Equal(t, "Bearer refreshed-connection", req.Header.Get("Authorization"), "do not mutate the SDK request")
		})
	}
}

func TestCompositeRequestTransportStripsCrossOriginCredentials(t *testing.T) {
	for _, target := range []string{
		"https://other.example/component",
		"https://sub.obot.example/component",
		"https://obot.example:8443/component",
		"http://obot.example/component",
	} {
		t.Run(target, func(t *testing.T) {
			ctx := component.ContextWithRequestHeaders(t.Context(), http.Header{"Authorization": {"Bearer current-caller"}})
			initial, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://obot.example/mcp-connect/component", nil)
			require.NoError(t, err)
			redirected, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
			require.NoError(t, err)
			redirected.Response = &http.Response{Request: initial}
			redirected.Header.Set("Authorization", "Bearer refreshed-connection")
			var got string
			transport := compositeRequestTransport{base: compositeRoundTripFunc(func(outbound *http.Request) (*http.Response, error) {
				got = outbound.Header.Get("Authorization")
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
			})}
			resp, err := transport.RoundTrip(redirected)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Empty(t, got, "neither the current nor retained token may cross origins")
		})
	}
}

func TestCompositeHTTPClientHandlesRedirects(t *testing.T) {
	for _, crossOrigin := range []bool{false, true} {
		t.Run(fmt.Sprint(crossOrigin), func(t *testing.T) {
			received := make(chan string, 1)
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				received <- req.Header.Get("Authorization")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer destination.Close()
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/start" {
					target := "/finish"
					if crossOrigin {
						target = destination.URL
					}
					http.Redirect(w, req, target, http.StatusTemporaryRedirect)
					return
				}
				received <- req.Header.Get("Authorization")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer gateway.Close()
			ctx := component.ContextWithRequestHeaders(t.Context(), http.Header{"Authorization": {"Bearer current-caller"}})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/start", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer retained-connection")
			resp, err := newCompositeHTTPClient().Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			want := "Bearer current-caller"
			if crossOrigin {
				want = ""
			}
			require.Equal(t, want, <-received)
		})
	}
}

func TestCompositeHTTPClientConcurrentCallers(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, req.Header.Get("Authorization"))
	}))
	defer gateway.Close()
	client := newCompositeHTTPClient()
	var calls errgroup.Group
	for i := range 20 {
		calls.Go(func() error {
			want := fmt.Sprintf("Bearer caller-%d", i)
			ctx := component.ContextWithRequestHeaders(t.Context(), http.Header{"Authorization": {want}})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer retained-connection")
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if string(body) != want {
				return fmt.Errorf("received %q, want %q", body, want)
			}
			return nil
		})
	}
	require.NoError(t, calls.Wait())
}
