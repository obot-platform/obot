package imagepullsecrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAvailability(t *testing.T) {
	tests := []struct {
		name      string
		backend   string
		static    []string
		available bool
	}{
		{name: "kubernetes with no static secrets", backend: "kubernetes", available: true},
		{name: "k8s alias", backend: "k8s", available: true},
		{name: "empty static names do not disable", backend: "kubernetes", static: []string{"", " "}, available: true},
		{name: "docker backend disabled", backend: "docker"},
		{name: "static secrets disabled", backend: "kubernetes", static: []string{"pull-secret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := Availability(tt.backend, tt.static)
			if capability.Available != tt.available {
				t.Fatalf("expected available=%v, got %v (%q)", tt.available, capability.Available, capability.Reason)
			}
			if !tt.available && capability.Reason == "" {
				t.Fatalf("expected disabled reason")
			}
		})
	}
}

func TestECRSubject(t *testing.T) {
	if got := ECRSubject(" obot ", " obot "); got != "system:serviceaccount:obot:obot" {
		t.Fatalf("unexpected ECR subject: %q", got)
	}
	if got := ECRSubject(" ", "obot"); got != "" {
		t.Fatalf("expected empty subject for empty namespace, got %q", got)
	}
	if got := ECRSubject("obot", " "); got != "" {
		t.Fatalf("expected empty subject for empty service account name, got %q", got)
	}
}

func TestNormalizeRegistryServer(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "ghcr.io", want: "ghcr.io"},
		{input: "https://GHCR.IO/", want: "ghcr.io"},
		{input: "LOCALHOST:5000", want: "localhost:5000"},
		{input: "127.0.0.1", want: "127.0.0.1"},
		{input: "127.0.0.1:5000", want: "127.0.0.1:5000"},
		{input: "[::1]", want: "[::1]"},
		{input: "https://[::1]:5000", want: "[::1]:5000"},
		{input: "", wantErr: true},
		{input: "   ", wantErr: true},
		{input: "ghcr.io foo", wantErr: true},
		{input: "ghcr.io/foo", wantErr: true},
		{input: "https://ghcr.io/owner", wantErr: true},
		{input: "https://user@ghcr.io", wantErr: true},
		{input: "https://", wantErr: true},
		{input: "ftp://ghcr.io", wantErr: true},
		{input: "bad_host", wantErr: true},
		{input: "ghcr.io:abc", wantErr: true},
		{input: "ghcr.io:0", wantErr: true},
		{input: "ghcr.io:", wantErr: true},
		{input: "ghcr.io:99999", wantErr: true},
		{input: "[::1]:", wantErr: true},
		{input: "2001:db8::1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeRegistryServer(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildDockerConfigJSON(t *testing.T) {
	configJSON, err := BuildDockerConfigJSON("https://GHCR.IO", "grant", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var config dockerConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatalf("failed to unmarshal docker config: %v", err)
	}

	auth, ok := config.Auths["ghcr.io"]
	if !ok {
		t.Fatalf("expected ghcr.io auth entry")
	}
	if auth.Username != "grant" || auth.Password != "secret" {
		t.Fatalf("unexpected auth entry: %#v", auth)
	}
	if auth.Auth != base64.StdEncoding.EncodeToString([]byte("grant:secret")) {
		t.Fatalf("unexpected auth value")
	}
}

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name            string
		image           string
		defaultRegistry string
		want            imageReference
		wantErr         bool
	}{
		{
			name:            "default registry and tag",
			image:           "team/app:1.0",
			defaultRegistry: "ghcr.io",
			want: imageReference{
				Registry:   "ghcr.io",
				Repository: "team/app",
				Reference:  "1.0",
			},
		},
		{
			name:            "explicit registry and digest",
			image:           "ghcr.io/team/app@sha256:abc",
			defaultRegistry: "example.com",
			want: imageReference{
				Registry:   "ghcr.io",
				Repository: "team/app",
				Reference:  "sha256:abc",
			},
		},
		{
			name:            "docker hub official image",
			image:           "nginx",
			defaultRegistry: "index.docker.io",
			want: imageReference{
				Registry:   "index.docker.io",
				Repository: "library/nginx",
				Reference:  "latest",
			},
		},
		{
			name:            "reject uppercase repository",
			image:           "Team/App:1.0",
			defaultRegistry: "ghcr.io",
			wantErr:         true,
		},
		{
			name:            "reject URL",
			image:           "https://ghcr.io/team/app:1.0",
			defaultRegistry: "ghcr.io",
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseImageReference(tt.image, tt.defaultRegistry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestBasicRegistryManifestBasicAuth(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v2/team/app/manifests/1.0" {
			http.NotFound(w, req)
			return
		}
		username, password, ok := req.BasicAuth()
		if !ok || username != "grant" || password != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := testBasicRegistryCredentials(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), "grant", "secret", "team/app:1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBasicRegistryManifestBearerAuth(t *testing.T) {
	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v2/team/app/manifests/1.0":
			if req.Header.Get("Authorization") == "Bearer registry-token" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/token",service="registry.test"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			username, password, ok := req.BasicAuth()
			if !ok || username != "grant" || password != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if req.URL.Query().Get("service") != "registry.test" {
				t.Errorf("expected service query, got %q", req.URL.RawQuery)
			}
			if req.URL.Query().Get("scope") != "repository:team/app:pull" {
				t.Errorf("expected manifest scope, got %q", req.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"registry-token"}`))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	result, err := testBasicRegistryCredentials(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), "grant", "secret", "team/app:1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBasicRegistryManifestBearerRejectsCrossRegistryRealm(t *testing.T) {
	tokenCalled := false
	tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenCalled = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer tokenServer.Close()

	registry := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v2/team/app/manifests/1.0":
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenServer.URL+`/token",service="registry.test"`)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, req)
		}
	}))
	defer registry.Close()

	_, err := testBasicRegistryCredentials(context.Background(), registry.Client(), strings.TrimPrefix(registry.URL, "https://"), "username", "secret", "team/app:1.0")
	if err == nil {
		t.Fatal("expected cross-registry token realm to be rejected")
	}
	if !strings.Contains(err.Error(), "token realm does not match") {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenCalled {
		t.Fatal("token endpoint was called for a cross-registry realm")
	}
}

func TestBasicRegistryManifestBearerRejectsHTTPRealm(t *testing.T) {
	var tokenCalled bool
	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v2/team/app/manifests/1.0":
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+strings.Replace(serverURL, "https://", "http://", 1)+`/token",service="registry.test"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			tokenCalled = true
			http.Error(w, "should not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	_, err := testBasicRegistryCredentials(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), "username", "secret", "team/app:1.0")
	if err == nil {
		t.Fatal("expected http token realm to be rejected")
	}
	if !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenCalled {
		t.Fatal("token endpoint was called for an http realm")
	}
}

func TestSanitizeNetworkErrorStripsConnectionDetails(t *testing.T) {
	err := sanitizeNetworkError(&url.Error{
		Op:  "Get",
		URL: "https://registry.example.com/v2/",
		Err: errors.New("dial tcp 10.96.0.1:443: connect: connection refused"),
	})

	if err.Error() != "registry request failed" {
		t.Fatalf("unexpected sanitized error: %q", err.Error())
	}
	if strings.Contains(err.Error(), "10.96.0.1") || strings.Contains(err.Error(), "443") {
		t.Fatalf("sanitized error leaked network details: %q", err.Error())
	}
}

func TestDockerConfigJSONManifestAccess(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v2/team/app/manifests/1.0" {
			http.NotFound(w, req)
			return
		}
		username, password, ok := req.BasicAuth()
		if !ok || username != "AWS" || password != "ecr-token" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry := strings.TrimPrefix(server.URL, "https://")
	configJSON, err := BuildDockerConfigJSON(server.URL, "AWS", "ecr-token")
	if err != nil {
		t.Fatalf("unexpected error building docker config: %v", err)
	}

	result, err := testDockerConfigJSONCredentials(context.Background(), server.Client(), configJSON, registry+"/team/app:1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestParseAuthParams(t *testing.T) {
	params := parseAuthParams(`realm="https://auth.example.com/token",service="registry.example.com",scope="repository:team/app:pull"`)
	if params["realm"] != "https://auth.example.com/token" {
		t.Fatalf("unexpected realm: %q", params["realm"])
	}
	if params["service"] != "registry.example.com" {
		t.Fatalf("unexpected service: %q", params["service"])
	}
	if params["scope"] != "repository:team/app:pull" {
		t.Fatalf("unexpected scope: %q", params["scope"])
	}
}

func TestParseAuthParamsPreservesQuotedCommasAndEscapes(t *testing.T) {
	params := parseAuthParams(`realm="https://auth.example.com/token?service=a,b",error="needs \"quoted\" value",service=registry.example.com`)
	if params["realm"] != "https://auth.example.com/token?service=a,b" {
		t.Fatalf("unexpected realm: %q", params["realm"])
	}
	if params["error"] != `needs "quoted" value` {
		t.Fatalf("unexpected error: %q", params["error"])
	}
	if params["service"] != "registry.example.com" {
		t.Fatalf("unexpected service: %q", params["service"])
	}
}

func TestParseAuthParamsPreservesEmptyQuotedValue(t *testing.T) {
	params := parseAuthParams(`realm="",service="registry.example.com"`)
	if realm, ok := params["realm"]; !ok || realm != "" {
		t.Fatalf("unexpected realm: value=%q ok=%t", realm, ok)
	}
	if params["service"] != "registry.example.com" {
		t.Fatalf("unexpected service: %q", params["service"])
	}
}

func TestValidateSpec(t *testing.T) {
	spec, err := ValidateSpec(v1.ImagePullSecretSpec{
		SecretName: "pull-secret",
		Type:       v1.ImagePullSecretTypeECR,
		ECR: &v1.ECRImagePullSecretSpec{
			RoleARN:   "arn:aws:iam::123456789012:role/obot-ecr",
			Region:    "us-east-1",
			IssuerURL: "https://issuer.example.com/",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.ECR.Audience != DefaultECRAudience {
		t.Fatalf("expected default audience, got %q", spec.ECR.Audience)
	}
	if spec.ECR.RefreshSchedule != DefaultECRRefreshSchedule {
		t.Fatalf("expected default refresh schedule, got %q", spec.ECR.RefreshSchedule)
	}
	if spec.ECR.IssuerURL != "https://issuer.example.com" {
		t.Fatalf("expected normalized issuer URL, got %q", spec.ECR.IssuerURL)
	}
}

func TestEffectiveSecretNames(t *testing.T) {
	deletionTime := metav1.Now()
	managed := []v1.ImagePullSecret{
		{Spec: v1.ImagePullSecretSpec{Enabled: true, SecretName: "managed-b"}},
		{Spec: v1.ImagePullSecretSpec{Enabled: false, SecretName: "disabled"}},
		{Spec: v1.ImagePullSecretSpec{Enabled: true, SecretName: "managed-a"}},
		{Spec: v1.ImagePullSecretSpec{Enabled: true, SecretName: "managed-a"}},
		{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTime},
			Spec:       v1.ImagePullSecretSpec{Enabled: true, SecretName: "deleting"},
		},
	}

	if got := EffectiveSecretNames([]string{"static-b", "static-a", "static-b"}, managed); !slices.Equal(got, []string{"static-a", "static-b"}) {
		t.Fatalf("unexpected static effective names: %v", got)
	}
	if got := EffectiveSecretNames(nil, managed); !slices.Equal(got, []string{"managed-a", "managed-b"}) {
		t.Fatalf("unexpected managed effective names: %v", got)
	}
}

func TestImagePullSecretsHashIgnoresOrder(t *testing.T) {
	first := Hash([]string{"b", "a", "b"})
	second := Hash([]string{"a", "b"})
	if first != second {
		t.Fatalf("expected stable hash, got %q and %q", first, second)
	}
}
