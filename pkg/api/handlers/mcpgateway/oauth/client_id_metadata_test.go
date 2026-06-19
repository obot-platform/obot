package oauth

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestCIMDHandler(rt roundTripFunc) *handler {
	return &handler{
		oauthConfig: handlers.OAuthAuthorizationServerConfig{
			ScopesSupported:                            []string{"profile"},
			ResponseTypesSupported:                     []string{"code"},
			GrantTypesSupported:                        []string{"authorization_code", "refresh_token"},
			TokenEndpointAuthMethodsSupported:          []string{"client_secret_basic", "client_secret_post", "private_key_jwt", "none"},
			TokenEndpointAuthSigningAlgValuesSupported: []string{"RS256"},
		},
		clientMetadataHTTPClient: &http.Client{Transport: rt},
		clientMetadataCache:      map[string]clientMetadataCacheEntry{},
	}
}

func TestValidateClientIDMetadataDocumentURL(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid", url: "https://client.example/oauth/client.json"},
		{name: "query is allowed", url: "https://client.example/oauth/client.json?v=1"},
		{name: "http rejected", url: "http://client.example/oauth/client.json", wantErr: true},
		{name: "missing path rejected", url: "https://client.example", wantErr: true},
		{name: "root path rejected", url: "https://client.example/", wantErr: true},
		{name: "fragment rejected", url: "https://client.example/oauth/client.json#frag", wantErr: true},
		{name: "userinfo rejected", url: "https://user@client.example/oauth/client.json", wantErr: true},
		{name: "dot segment rejected", url: "https://client.example/oauth/../client.json", wantErr: true},
		{name: "encoded dot segment rejected", url: "https://client.example/oauth/%2e%2e/client.json", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateClientIDMetadataDocumentURL(tt.url)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveClientIDMetadataDocument(t *testing.T) {
	t.Parallel()

	const clientID = "https://client.example/oauth/client.json"
	var requests int
	h := newTestCIMDHandler(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.String() != clientID {
			t.Fatalf("unexpected metadata URL: %s", req.URL.String())
		}
		if got := req.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected accept header: %s", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":  []string{"application/json"},
				"Cache-Control": []string{"max-age=60"},
			},
			Body: io.NopCloser(strings.NewReader(`{
				"client_id":"https://client.example/oauth/client.json",
				"client_name":"Example Client",
				"redirect_uris":["http://127.0.0.1:3000/callback"],
				"scope":"profile"
			}`)),
		}, nil
	})

	client, err := h.resolveClientIDMetadataDocument(context.Background(), clientID)
	if err != nil {
		t.Fatalf("resolve metadata: %v", err)
	}
	if client.Name != clientID {
		t.Fatalf("expected client name %q, got %q", clientID, client.Name)
	}
	if client.Spec.Manifest.ClientName != "Example Client" {
		t.Fatalf("unexpected client name: %s", client.Spec.Manifest.ClientName)
	}
	if client.Spec.Manifest.TokenEndpointAuthMethod != "none" {
		t.Fatalf("expected default token endpoint auth method none, got %q", client.Spec.Manifest.TokenEndpointAuthMethod)
	}
	if client.Spec.Manifest.ApplicationType != "web" {
		t.Fatalf("expected default application type web, got %q", client.Spec.Manifest.ApplicationType)
	}

	if _, err = h.resolveClientIDMetadataDocument(context.Background(), clientID); err != nil {
		t.Fatalf("resolve cached metadata: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected cached metadata to avoid second fetch, got %d requests", requests)
	}
}

func TestResolveClientIDMetadataDocumentValidation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "client id mismatch",
			body: `{"client_id":"https://other.example/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"]}`,
		},
		{
			name: "missing client name",
			body: `{"client_id":"https://client.example/oauth/client.json","redirect_uris":["http://127.0.0.1/callback"]}`,
		},
		{
			name: "missing redirect uris",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client"}`,
		},
		{
			name: "shared secret auth rejected",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"],"token_endpoint_auth_method":"client_secret_post"}`,
		},
		{
			name: "invalid application type rejected",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"],"application_type":"service"}`,
		},
		{
			name: "client secret rejected",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"],"client_secret":"secret"}`,
		},
		{
			name: "private key jwt requires keys",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"],"token_endpoint_auth_method":"private_key_jwt"}`,
		},
		{
			name: "jwks and jwks uri rejected",
			body: `{"client_id":"https://client.example/oauth/client.json","client_name":"Example Client","redirect_uris":["http://127.0.0.1/callback"],"token_endpoint_auth_method":"private_key_jwt","jwks_uri":"https://client.example/jwks.json","jwks":{"keys":[]}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestCIMDHandler(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			})

			if _, err := h.resolveClientIDMetadataDocument(context.Background(), "https://client.example/oauth/client.json"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestResolveClientIDMetadataDocumentPrivateKeyJWT(t *testing.T) {
	t.Parallel()

	const clientID = "https://client.example/oauth/client.json"
	h := newTestCIMDHandler(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"client_id":"https://client.example/oauth/client.json",
				"client_name":"Example Client",
				"redirect_uris":["http://127.0.0.1/callback"],
				"token_endpoint_auth_method":"private_key_jwt",
				"jwks":{"keys":[]}
			}`)),
		}, nil
	})

	client, err := h.resolveClientIDMetadataDocument(context.Background(), clientID)
	if err != nil {
		t.Fatalf("resolve metadata: %v", err)
	}
	if client.Spec.Manifest.TokenEndpointAuthMethod != "private_key_jwt" {
		t.Fatalf("expected private_key_jwt, got %q", client.Spec.Manifest.TokenEndpointAuthMethod)
	}
	if client.Spec.Manifest.JWKS == "" {
		t.Fatal("expected raw jwks to be retained")
	}
}

func TestResolveClientIDMetadataDocumentNativeApplicationType(t *testing.T) {
	t.Parallel()

	const clientID = "https://client.example/oauth/client.json"
	h := newTestCIMDHandler(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"client_id":"https://client.example/oauth/client.json",
				"client_name":"Example Client",
				"redirect_uris":["http://127.0.0.1/callback"],
				"application_type":"native"
			}`)),
		}, nil
	})

	client, err := h.resolveClientIDMetadataDocument(context.Background(), clientID)
	if err != nil {
		t.Fatalf("resolve metadata: %v", err)
	}
	if client.Spec.Manifest.ApplicationType != "native" {
		t.Fatalf("expected native application type, got %q", client.Spec.Manifest.ApplicationType)
	}
}

func TestClientIDNativeExceptionsDefaultToNative(t *testing.T) {
	t.Parallel()

	h := newTestCIMDHandler(nil)
	for clientID := range clientIDNativeExceptions {
		t.Run(clientID, func(t *testing.T) {
			t.Parallel()

			client, err := h.oauthClientFromMetadataDocument(clientID, clientIDMetadataDocument{
				OAuthClientManifest: types.OAuthClientManifest{
					ClientName:   "Example Client",
					RedirectURIs: []string{"http://127.0.0.1/callback"},
				},
				ClientID: clientID,
			})
			if err != nil {
				t.Fatalf("resolve metadata: %v", err)
			}
			if client.Spec.Manifest.ApplicationType != "native" {
				t.Fatalf("expected native application type, got %q", client.Spec.Manifest.ApplicationType)
			}
		})
	}
}

func TestFetchClientIDMetadataDocumentRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	h := newTestCIMDHandler(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(strings.Repeat(" ", maxClientIDMetadataDocumentBytes+1))),
		}, nil
	})

	if _, _, err := h.fetchClientIDMetadataDocument(context.Background(), "https://client.example/oauth/client.json"); err == nil {
		t.Fatal("expected oversized metadata error")
	}
}

func TestClientMetadataCacheExpiration(t *testing.T) {
	t.Parallel()

	expiration := clientMetadataCacheExpiration(http.Header{"Cache-Control": []string{"max-age=30"}})
	if time.Until(expiration) <= 0 {
		t.Fatalf("expected future expiration, got %v", expiration)
	}

	expiration = clientMetadataCacheExpiration(http.Header{"Cache-Control": []string{"no-store, max-age=30"}})
	if !expiration.IsZero() {
		t.Fatalf("expected no-store to disable caching, got %v", expiration)
	}

	expiration = clientMetadataCacheExpiration(http.Header{"Cache-Control": []string{"max-age=30, no-store"}})
	if !expiration.IsZero() {
		t.Fatalf("expected later no-store to disable caching, got %v", expiration)
	}

	expiration = clientMetadataCacheExpiration(http.Header{"Cache-Control": []string{"max-age=30, no-cache"}})
	if !expiration.IsZero() {
		t.Fatalf("expected later no-cache to disable caching, got %v", expiration)
	}

	expiration = clientMetadataCacheExpiration(http.Header{"Cache-Control": []string{"max-age=30", "no-store"}})
	if !expiration.IsZero() {
		t.Fatalf("expected no-store in second cache-control header to disable caching, got %v", expiration)
	}
}

func TestCacheClientMetadataEvictsOldestEntryAtLimit(t *testing.T) {
	t.Parallel()

	h := newTestCIMDHandler(nil)
	now := time.Now()
	for i := range clientMetadataCacheMaxEntries {
		clientID := "https://client.example/client-" + strconv.Itoa(i) + ".json"
		h.clientMetadataCache[clientID] = clientMetadataCacheEntry{
			client:    v1.OAuthClient{ObjectMeta: metav1.ObjectMeta{Name: clientID}},
			expiresAt: now.Add(time.Hour),
			cachedAt:  now.Add(time.Duration(i) * time.Second),
		}
	}

	const oldestClientID = "https://client.example/client-0.json"
	const newClientID = "https://client.example/new-client.json"
	h.cacheClientMetadata(newClientID, v1.OAuthClient{ObjectMeta: metav1.ObjectMeta{Name: newClientID}}, now.Add(time.Hour))

	if len(h.clientMetadataCache) != clientMetadataCacheMaxEntries {
		t.Fatalf("expected cache size %d, got %d", clientMetadataCacheMaxEntries, len(h.clientMetadataCache))
	}
	if _, ok := h.clientMetadataCache[oldestClientID]; ok {
		t.Fatalf("expected oldest entry %q to be evicted", oldestClientID)
	}
	if _, ok := h.clientMetadataCache[newClientID]; !ok {
		t.Fatalf("expected new entry %q to be cached", newClientID)
	}
}
