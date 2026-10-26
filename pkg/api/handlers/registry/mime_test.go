package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestGuessMimeType(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want string
	}{
		{
			url:  "https://example.com/icon.PNG?version=2#preview",
			want: "image/png",
		},
		{
			url:  "https://example.com/icon.jpg",
			want: "image/jpeg",
		},
		{
			url:  "https://example.com/icon.jpeg",
			want: "image/jpeg",
		},
		{
			url:  "https://example.com/icon.svg",
			want: "image/svg+xml",
		},
		{
			url:  "https://example.com/icon.webp",
			want: "image/webp",
		},
		{
			url: "https://example.com/icon?filename=icon.png",
		},
		{
			url: "https://example.com/icon.gif",
		},
	} {
		t.Run(tc.url, func(t *testing.T) {
			if got := guessMimeType(tc.url); got != tc.want {
				t.Fatalf("MIME type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegistryDiscoveryDoesNotFetchIcons(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
	}))
	defer server.Close()

	definition := registryDefinition("vmcp1example", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	definition.Spec.Manifest.Icon = server.URL + "/icon"
	storage := newRegistryTestStorage(definition)
	for _, route := range []string{"/v0.1/servers", registryDetailPath(definition.Name) + "/versions/latest"} {
		response := registryRequest(t, storage, nil, route)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", route, response.Code)
		}
		if strings.Contains(response.Body.String(), `"mimeType"`) || !strings.Contains(response.Body.String(), definition.Spec.Manifest.Icon) {
			t.Fatalf("expected icon URL without MIME override: %s", response.Body.String())
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("discovery fetched icons: %d requests", hits.Load())
	}
}
