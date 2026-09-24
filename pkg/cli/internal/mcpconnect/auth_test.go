package mcpconnect

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/require"
)

func TestOAuthRegistrationRelayAndTokenReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var gatewayURL, registeredRedirect, clientState, challenge string
	var registrations, exchanges, relays atomic.Int32
	gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp-connect/test":
			require.Equal(t, http.MethodGet, r.Method)
			if r.Header.Get("Authorization") == "Bearer obot-access" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+gatewayURL+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource":
			fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, gatewayURL+"/mcp-connect/test", gatewayURL)
		case "/.well-known/oauth-authorization-server":
			fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"registration_endpoint":%q,"response_types_supported":["code"],"grant_types_supported":["authorization_code","refresh_token"],"code_challenge_methods_supported":["S256"],"token_endpoint_auth_methods_supported":["none"]}`, gatewayURL, gatewayURL+"/authorize", gatewayURL+"/token", gatewayURL+"/register")
		case "/register":
			var metadata oauthex.ClientRegistrationMetadata
			require.NoError(t, json.NewDecoder(r.Body).Decode(&metadata))
			require.Len(t, metadata.RedirectURIs, 1)
			registeredRedirect = metadata.RedirectURIs[0]
			require.Equal(t, "none", metadata.TokenEndpointAuthMethod)
			registrations.Add(1)
			fmt.Fprintf(w, `{"client_id":"local-client","redirect_uris":[%q],"token_endpoint_auth_method":"none"}`, registeredRedirect)
		case "/authorize":
			require.Equal(t, registeredRedirect, r.URL.Query().Get("redirect_uri"))
			require.Equal(t, "S256", r.URL.Query().Get("code_challenge_method"))
			challenge = r.URL.Query().Get("code_challenge")
			clientState = r.URL.Query().Get("state")
			u, err := url.Parse(registeredRedirect)
			require.NoError(t, err)
			u.Path = "/provider/callback"
			u.RawQuery = "code=upstream-code&state=upstream-state"
			http.Redirect(w, r, u.String(), http.StatusFound)
		case "/oauth/mcp/callback":
			require.Equal(t, "upstream-code", r.URL.Query().Get("code"))
			require.Equal(t, "upstream-state", r.URL.Query().Get("state"))
			relays.Add(1)
			http.Redirect(w, r, registeredRedirect+"?code=obot-code&state="+url.QueryEscape(clientState), http.StatusFound)
		case "/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, registeredRedirect, r.Form.Get("redirect_uri"))
			require.Equal(t, "obot-code", r.Form.Get("code"))
			require.Equal(t, "local-client", r.Form.Get("client_id"))
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			require.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(hash[:]))
			exchanges.Add(1)
			fmt.Fprint(w, `{"access_token":"obot-access","refresh_token":"obot-refresh","token_type":"Bearer","expires_in":3600}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()
	gatewayURL = gateway.URL
	parsed, err := url.Parse(gatewayURL)
	require.NoError(t, err)
	paths, err := allowedCallbackPaths([]string{"/provider/callback"})
	require.NoError(t, err)
	callback := &callbackHandler{gateway: parsed, providerPaths: paths, openBrowser: func(u string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := gateway.Client().Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("callback returned %d", resp.StatusCode)
		}
		return nil
	}}
	listener := httptest.NewServer(callback)
	defer listener.Close()
	redirect := listener.URL + obotCallbackPath
	dir := t.TempDir()
	endpoint := gateway.URL + "/mcp-connect/test"
	handler, err := newOAuthHandler(ctx, dir, endpoint, redirect, callback, gateway.Client())
	require.NoError(t, err)
	require.NoError(t, authenticate(ctx, endpoint, handler, gateway.Client()))
	require.EqualValues(t, 1, registrations.Load())
	require.EqualValues(t, 1, exchanges.Load())
	require.EqualValues(t, 1, relays.Load())
	require.Equal(t, redirect, registeredRedirect)
	// A new process/listener reuses the Obot token without registering or opening a browser.
	again, err := newOAuthHandler(ctx, dir, endpoint, "http://localhost:9999"+obotCallbackPath, callback, gateway.Client())
	require.NoError(t, err)
	require.NoError(t, authenticate(ctx, endpoint, again, gateway.Client()))
	source, err := again.TokenSource(ctx)
	require.NoError(t, err)
	require.NotNil(t, source)
	token, err := source.Token()
	require.NoError(t, err)
	require.Equal(t, "obot-access", token.AccessToken)
	require.EqualValues(t, 1, registrations.Load())
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	info, err := os.Stat(entries[0])
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	cached, err := os.ReadFile(entries[0])
	require.NoError(t, err)
	require.False(t, strings.Contains(string(cached), "upstream-code"))
	other := newTokenStore(dir, gateway.URL+"/mcp-connect/other")
	source, err = other.load(ctx, gateway.Client())
	require.NoError(t, err)
	require.Nil(t, source)
}
