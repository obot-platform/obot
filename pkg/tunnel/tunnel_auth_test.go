package tunnel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeTunnelReader struct {
	tunnels   map[string]v1.MCPTunnel
	err       error
	getCalls  int
	listCalls int
	selector  fields.Selector
}

func (f *fakeTunnelReader) Get(_ context.Context, key kclient.ObjectKey, obj kclient.Object, _ ...kclient.GetOption) error {
	f.getCalls++
	if f.err != nil {
		return f.err
	}
	stored, ok := f.tunnels[key.Name]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{
			Group:    "obot.obot.ai",
			Resource: "mcptunnels",
		}, key.Name)
	}
	*obj.(*v1.MCPTunnel) = stored
	return nil
}

func (f *fakeTunnelReader) List(_ context.Context, obj kclient.ObjectList, opts ...kclient.ListOption) error {
	f.listCalls++
	options := &kclient.ListOptions{}
	options.ApplyOptions(opts)
	f.selector = options.FieldSelector
	if f.err != nil {
		return f.err
	}
	list, ok := obj.(*v1.MCPTunnelList)
	if !ok {
		return errors.New("unexpected tunnel list type")
	}
	list.Items = list.Items[:0]
	for _, tunnel := range f.tunnels {
		if f.selector != nil && !f.selector.Matches(fields.Set{
			v1.MCPTunnelCredentialIDField: tunnel.Spec.CredentialID,
		}) {
			continue
		}
		list.Items = append(list.Items, tunnel)
	}
	return nil
}

func TestTunnelAuthenticator(t *testing.T) {
	validToken, credential, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	otherToken, otherCredential, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}

	newStoredTunnel := func(name, storedCredential string) v1.MCPTunnel {
		tunnel := v1.MCPTunnel{}
		tunnel.Name = name
		tunnel.Spec.Credential = storedCredential
		credentialID, err := CredentialID(storedCredential)
		if err != nil {
			t.Fatal(err)
		}
		tunnel.Spec.CredentialID = credentialID
		return tunnel
	}

	const (
		tunnelName      = "mt1office"
		otherTunnelName = "mt1warehouse"
	)
	storedTunnel := newStoredTunnel(tunnelName, credential)
	otherStoredTunnel := newStoredTunnel(otherTunnelName, otherCredential)

	tests := []struct {
		name           string
		authorization  string
		path           string
		tunnels        map[string]v1.MCPTunnel
		readerErr      error
		wantOK         bool
		wantList       bool
		wantTunnelName string
		wantErr        bool
	}{
		{
			name:           "valid token",
			authorization:  "Bearer " + validToken,
			path:           "/tunnel/connect",
			tunnels:        map[string]v1.MCPTunnel{tunnelName: storedTunnel},
			wantOK:         true,
			wantList:       true,
			wantTunnelName: tunnelName,
		},
		{
			name:          "selects matching tunnel among multiple tunnels",
			authorization: "Bearer " + otherToken,
			path:          "/tunnel/connect",
			tunnels: map[string]v1.MCPTunnel{
				tunnelName:      storedTunnel,
				otherTunnelName: otherStoredTunnel,
			},
			wantOK:         true,
			wantList:       true,
			wantTunnelName: otherTunnelName,
		},
		{
			name:          "well formed token does not match",
			authorization: "Bearer " + otherToken,
			path:          "/tunnel/connect",
			tunnels:       map[string]v1.MCPTunnel{tunnelName: storedTunnel},
			wantList:      true,
		},
		{
			name:          "malformed token",
			authorization: "Bearer wrong",
			path:          "/tunnel/connect",
		},
		{
			name:          "no tunnels",
			authorization: "Bearer " + validToken,
			path:          "/tunnel/connect",
			wantList:      true,
		},
		{
			name: "no authorization",
			path: "/tunnel/connect",
		},
		{
			name:          "non-bearer authorization",
			authorization: "Basic abc123",
			path:          "/tunnel/connect",
		},
		{
			name:          "empty bearer token",
			authorization: "Bearer ",
			path:          "/tunnel/connect",
		},
		{
			name:          "bearer on other route",
			authorization: "Bearer " + validToken,
			path:          "/api/mcp-tunnels",
		},
		{
			name:          "old named tunnel path",
			authorization: "Bearer " + validToken,
			path:          "/tunnel/connect/" + tunnelName,
		},
		{
			name:          "nested tunnel path",
			authorization: "Bearer " + validToken,
			path:          "/tunnel/connect/" + tunnelName + "/extra",
		},
		{
			name:          "storage error",
			authorization: "Bearer " + validToken,
			path:          "/tunnel/connect",
			readerErr:     errors.New("storage unavailable"),
			wantList:      true,
			wantErr:       true,
		},
		{
			name:          "duplicate credentials are rejected",
			authorization: "Bearer " + validToken,
			path:          "/tunnel/connect",
			tunnels: map[string]v1.MCPTunnel{
				tunnelName:      storedTunnel,
				otherTunnelName: newStoredTunnel(otherTunnelName, credential),
			},
			wantList: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeTunnelReader{
				tunnels: tt.tunnels,
				err:     tt.readerErr,
			}
			authenticator := &Authenticator{tunnels: reader}
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			response, ok, err := authenticator.AuthenticateRequest(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AuthenticateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Fatalf("AuthenticateRequest() ok = %v, want %v", ok, tt.wantOK)
			}
			if (reader.listCalls > 0) != tt.wantList {
				t.Fatalf("tunnel list = %v, want %v", reader.listCalls > 0, tt.wantList)
			}
			if tt.wantList && reader.selector == nil {
				t.Fatal("tunnel list did not include a credential field selector")
			}
			if reader.getCalls != 0 {
				t.Fatalf("tunnel Get calls = %d, want 0", reader.getCalls)
			}
			if tt.wantErr {
				return
			}
			if tt.wantTunnelName == "" {
				if response != nil {
					t.Fatalf("AuthenticateRequest() response = %#v, want nil", response)
				}
				return
			}

			if response == nil || response.User == nil {
				t.Fatal("AuthenticateRequest() returned no user")
			}
			if response.User.GetName() != tt.wantTunnelName || response.User.GetUID() != tt.wantTunnelName {
				t.Fatalf("principal = %q/%q, want %q", response.User.GetName(), response.User.GetUID(), tt.wantTunnelName)
			}
			groups := response.User.GetGroups()
			if len(groups) != 1 || groups[0] != types.GroupTunnel {
				t.Fatalf("groups = %#v, want only %q", groups, types.GroupTunnel)
			}
		})
	}
}
