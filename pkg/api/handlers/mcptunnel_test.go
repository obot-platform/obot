package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	tunnelpkg "github.com/obot-platform/obot/pkg/tunnel"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mcpTunnelTestStorage struct {
	kclient.WithWatch
	next int
}

type failingMCPTunnelDeleteStorage struct {
	kclient.WithWatch
}

func (*failingMCPTunnelDeleteStorage) Delete(context.Context, kclient.Object, ...kclient.DeleteOption) error {
	return fmt.Errorf("delete failed")
}

func (s *mcpTunnelTestStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if obj.GetName() == "" {
		s.next++
		obj.SetName(fmt.Sprintf("%stest-%d", obj.GetGenerateName(), s.next))
	}
	return s.WithWatch.Create(ctx, obj, opts...)
}

type mcpTunnelTestCloser struct {
	names         []string
	credentialIDs []string
}

func (c *mcpTunnelTestCloser) DisconnectCredential(name, credentialID string) {
	c.names = append(c.names, name)
	c.credentialIDs = append(c.credentialIDs, credentialID)
}

func TestMCPTunnelHandlerLifecycle(t *testing.T) {
	storage := newMCPTunnelTestStorage()
	closer := &mcpTunnelTestCloser{}
	handler := NewMCPTunnelHandler(closer)

	created, recorder, err := callMCPTunnelHandler(t, storage, http.MethodPost, "/api/mcp-tunnels", "", `{
		"displayName":" Office ",
		"description":" Private services ",
		"allowedURLs":[" https://api.internal/* ","*.corp.internal"]
	}`, handler.Create)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("Create() status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if !strings.HasPrefix(created.ID, system.MCPTunnelPrefix) {
		t.Fatalf("created ID = %q, want prefix %q", created.ID, system.MCPTunnelPrefix)
	}
	if created.Manifest.DisplayName != "Office" ||
		created.Manifest.Description != "Private services" ||
		len(created.Manifest.AllowedURLs) != 2 ||
		created.Manifest.AllowedURLs[0] != "https://api.internal/*" {
		t.Fatalf("created manifest was not normalized: %#v", created.Manifest)
	}
	createdRaw := decodeFullMCPTunnelToken(t, created.Token)

	var stored v1.MCPTunnel
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      created.ID,
	}, &stored); err != nil {
		t.Fatalf("failed to get stored tunnel: %v", err)
	}
	originalCredential := stored.Spec.Credential
	if originalCredential == created.Token {
		t.Fatal("stored credential contains the recoverable token")
	}
	if !tunnelpkg.CredentialMatches(originalCredential, created.Token) {
		t.Fatal("stored credential does not match create token")
	}
	originalCredentialID, err := tunnelpkg.CredentialID(originalCredential)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Spec.CredentialID != originalCredentialID {
		t.Fatalf("stored credential ID = %q, want %q", stored.Spec.CredentialID, originalCredentialID)
	}

	got, _, err := callMCPTunnelHandler(t, storage, http.MethodGet, "/api/mcp-tunnels/"+created.ID, created.ID, "", handler.Get)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertMCPTunnelPreview(t, got.Token, createdRaw)

	list, err := callMCPTunnelListHandler(t, storage, handler.List)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("List() items = %#v, want tunnel %q", list.Items, created.ID)
	}
	assertMCPTunnelPreview(t, list.Items[0].Token, createdRaw)

	updated, _, err := callMCPTunnelHandler(t, storage, http.MethodPut, "/api/mcp-tunnels/"+created.ID, created.ID, `{
		"displayName":"Branch Office",
		"description":"Updated",
		"allowedURLs":["internal.example.com"]
	}`, handler.Update)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Manifest.DisplayName != "Branch Office" {
		t.Fatalf("Update() displayName = %q", updated.Manifest.DisplayName)
	}
	assertMCPTunnelPreview(t, updated.Token, createdRaw)
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      created.ID,
	}, &stored); err != nil {
		t.Fatalf("failed to get updated tunnel: %v", err)
	}
	if stored.Spec.Credential != originalCredential {
		t.Fatal("Update() changed the tunnel credential")
	}

	rotated, _, err := callMCPTunnelHandler(t, storage, http.MethodPost, "/api/mcp-tunnels/"+created.ID+"/rotate-secret", created.ID, "", handler.RotateSecret)
	if err != nil {
		t.Fatalf("RotateSecret() error = %v", err)
	}
	rotatedRaw := decodeFullMCPTunnelToken(t, rotated.Token)
	if bytes.Equal(rotatedRaw, createdRaw) {
		t.Fatal("RotateSecret() returned the original token")
	}
	if len(closer.names) != 1 || closer.names[0] != created.ID {
		t.Fatalf("Disconnect() names = %#v, want [%q]", closer.names, created.ID)
	}
	if len(closer.credentialIDs) != 1 || closer.credentialIDs[0] != originalCredentialID {
		t.Fatalf("Disconnect() credential IDs = %#v, want [%q]", closer.credentialIDs, originalCredentialID)
	}
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      created.ID,
	}, &stored); err != nil {
		t.Fatalf("failed to get rotated tunnel: %v", err)
	}
	if !tunnelpkg.CredentialMatches(stored.Spec.Credential, rotated.Token) {
		t.Fatal("stored credential does not match rotated token")
	}
	if tunnelpkg.CredentialMatches(stored.Spec.Credential, created.Token) {
		t.Fatal("stored credential still matches original token")
	}
	rotatedCredentialID, err := tunnelpkg.CredentialID(stored.Spec.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Spec.CredentialID != rotatedCredentialID {
		t.Fatalf("rotated credential ID = %q, want %q", stored.Spec.CredentialID, rotatedCredentialID)
	}
	if rotatedCredentialID == originalCredentialID {
		t.Fatal("RotateSecret() retained the original credential label")
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/mcp-tunnels/"+created.ID, nil)
	deleteRequest.SetPathValue("id", created.ID)
	deleteRecorder := httptest.NewRecorder()
	if err := handler.Delete(api.Context{
		ResponseWriter: deleteRecorder,
		Request:        deleteRequest,
		Storage:        storage,
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("Delete() status = %d, want %d", deleteRecorder.Code, http.StatusOK)
	}
	var deleted map[string]string
	if err := json.NewDecoder(deleteRecorder.Body).Decode(&deleted); err != nil {
		t.Fatalf("failed to decode Delete() response: %v", err)
	}
	if deleted["deleted"] != created.ID {
		t.Fatalf("Delete() response = %#v, want deleted %q", deleted, created.ID)
	}
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      created.ID,
	}, &stored); !apierrors.IsNotFound(err) {
		t.Fatalf("Get() after Delete() error = %v, want not found", err)
	}
	if len(closer.names) != 2 || closer.names[1] != created.ID {
		t.Fatalf("Disconnect() names after Delete() = %#v, want [%q %q]", closer.names, created.ID, created.ID)
	}
	if len(closer.credentialIDs) != 2 || closer.credentialIDs[1] != rotatedCredentialID {
		t.Fatalf("Disconnect() credential IDs after Delete() = %#v, want second %q", closer.credentialIDs, rotatedCredentialID)
	}
}

func TestMCPTunnelHandlerDeleteDoesNotDisconnectOnFailure(t *testing.T) {
	const tunnelName = "mt1office"
	baseStorage := newMCPTunnelTestStorage(&v1.MCPTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tunnelName,
			Namespace: system.DefaultNamespace,
		},
	})
	storage := &failingMCPTunnelDeleteStorage{WithWatch: baseStorage.WithWatch}
	closer := &mcpTunnelTestCloser{}
	handler := NewMCPTunnelHandler(closer)

	request := httptest.NewRequest(http.MethodDelete, "/api/mcp-tunnels/"+tunnelName, nil)
	request.SetPathValue("id", tunnelName)
	err := handler.Delete(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
	})
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("Delete() error = %v, want delete failure", err)
	}
	if len(closer.names) != 0 {
		t.Fatalf("Disconnect() names = %#v, want none", closer.names)
	}
}

func TestMCPTunnelHandlerUpdatePreservesCatalogEntryTargets(t *testing.T) {
	const tunnelName = "mt1office"
	_, credential, err := tunnelpkg.NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := tunnelpkg.CredentialID(credential)
	if err != nil {
		t.Fatal(err)
	}
	storage := newMCPTunnelTestStorage(
		&v1.MCPTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tunnelName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPTunnelSpec{
				Manifest: types.MCPTunnelManifest{
					DisplayName: "Office",
					AllowedURLs: []string{
						"https://accounting.internal/*",
						"https://operations.internal/*",
					},
				},
				Credential:   credential,
				CredentialID: credentialID,
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp1z-accounting",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Accounting MCP",
					Runtime: types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{
						FixedURL:   "https://accounting.internal/mcp",
						TunnelName: tunnelName,
					},
				},
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp1a-composite",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Operations Composite",
					Runtime: types.RuntimeComposite,
					CompositeConfig: &types.CompositeCatalogConfig{
						ComponentServers: []types.CatalogComponentServer{{
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteCatalogConfig{
									FixedURL:   "https://operations.internal/mcp",
									TunnelName: tunnelName,
								},
							},
						}},
					},
				},
			},
		},
	)
	handler := NewMCPTunnelHandler(nil)

	_, _, err = callMCPTunnelHandler(t, storage, http.MethodPut, "/api/mcp-tunnels/"+tunnelName, tunnelName, `{
		"displayName":"Office",
		"allowedURLs":["https://replacement.internal/mcp"]
	}`, handler.Update)
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) {
		t.Fatalf("Update() error = %v, want ErrHTTP", err)
	}
	if errHTTP.Code != http.StatusBadRequest {
		t.Fatalf("Update() status = %d, want %d", errHTTP.Code, http.StatusBadRequest)
	}
	const wantMessagePart = `MCP tunnel "Office" cannot be updated`
	if !strings.Contains(errHTTP.Message, wantMessagePart) {
		t.Fatalf("Update() message = %q, want containing %q", errHTTP.Message, wantMessagePart)
	}

	var stored v1.MCPTunnel
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      tunnelName,
	}, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Spec.Manifest.AllowedURLs) != 2 {
		t.Fatalf("Update() changed allowedURLs despite catalog entry references: %#v", stored.Spec.Manifest.AllowedURLs)
	}

	updated, _, err := callMCPTunnelHandler(t, storage, http.MethodPut, "/api/mcp-tunnels/"+tunnelName, tunnelName, `{
		"displayName":"Updated Office",
		"allowedURLs":["*.internal"]
	}`, handler.Update)
	if err != nil {
		t.Fatalf("Update() with replacement pattern error = %v", err)
	}
	if updated.Manifest.DisplayName != "Updated Office" {
		t.Fatalf("Update() displayName = %q, want Updated Office", updated.Manifest.DisplayName)
	}
	if len(updated.Manifest.AllowedURLs) != 1 || updated.Manifest.AllowedURLs[0] != "*.internal" {
		t.Fatalf("Update() allowedURLs = %#v, want [*.internal]", updated.Manifest.AllowedURLs)
	}
}

func TestMCPTunnelHandlerDeleteBlockedByCatalogEntries(t *testing.T) {
	const tunnelName = "mt1office"
	storage := newMCPTunnelTestStorage(
		&v1.MCPTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tunnelName,
				Namespace: system.DefaultNamespace,
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp1z-accounting",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Accounting MCP",
					Runtime: types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{
						FixedURL:   "https://accounting.internal/mcp",
						TunnelName: tunnelName,
					},
				},
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp1a-composite",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Operations Composite",
					Runtime: types.RuntimeComposite,
					CompositeConfig: &types.CompositeCatalogConfig{
						ComponentServers: []types.CatalogComponentServer{{
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteCatalogConfig{
									FixedURL:   "https://operations.internal/mcp",
									TunnelName: tunnelName,
								},
							},
						}},
					},
				},
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp1unrelated",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Unrelated MCP",
					Runtime: types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{
						FixedURL:   "https://unrelated.internal/mcp",
						TunnelName: "mt1warehouse",
					},
				},
			},
		},
	)
	closer := &mcpTunnelTestCloser{}
	handler := NewMCPTunnelHandler(closer)

	request := httptest.NewRequest(http.MethodDelete, "/api/mcp-tunnels/"+tunnelName, nil)
	request.SetPathValue("id", tunnelName)
	err := handler.Delete(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
	})

	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) {
		t.Fatalf("Delete() error = %v, want ErrHTTP", err)
	}
	if errHTTP.Code != http.StatusBadRequest {
		t.Fatalf("Delete() status = %d, want %d", errHTTP.Code, http.StatusBadRequest)
	}
	const wantMessagePart = `MCP tunnel "mt1office" cannot be deleted`
	if !strings.Contains(errHTTP.Message, wantMessagePart) {
		t.Fatalf("Delete() message = %q, want containing %q", errHTTP.Message, wantMessagePart)
	}

	var stored v1.MCPTunnel
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      tunnelName,
	}, &stored); err != nil {
		t.Fatalf("tunnel was deleted despite catalog entry references: %v", err)
	}
	if len(closer.names) != 0 {
		t.Fatalf("Disconnect() names = %#v, want none", closer.names)
	}
}

func TestReadAndValidateMCPTunnelManifest(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid",
			body: `{"displayName":"Office","allowedURLs":["https://example.com/*","*.internal"]}`,
		},
		{
			name:    "missing display name",
			body:    `{"allowedURLs":["https://example.com"]}`,
			wantErr: "displayName is required",
		},
		{
			name:    "empty allowed URL",
			body:    `{"displayName":"Office","allowedURLs":[" "]}`,
			wantErr: "must not be empty",
		},
		{
			name:    "middle wildcard",
			body:    `{"displayName":"Office","allowedURLs":["api.*.internal"]}`,
			wantErr: "beginning or end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/mcp-tunnels", strings.NewReader(tt.body))
			manifest, err := readAndValidateMCPTunnelManifest(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        request,
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("readAndValidateMCPTunnelManifest() error = %v", err)
				}
				if manifest.DisplayName != "Office" {
					t.Fatalf("displayName = %q, want Office", manifest.DisplayName)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("readAndValidateMCPTunnelManifest() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func newMCPTunnelTestStorage(objects ...kclient.Object) *mcpTunnelTestStorage {
	return &mcpTunnelTestStorage{
		WithWatch: fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(objects...).
			WithIndex(&v1.MCPServerCatalogEntry{}, "spec.manifest.runtime", mcpServerCatalogEntryIndex("spec.manifest.runtime")).
			WithIndex(&v1.MCPServerCatalogEntry{}, "spec.manifest.remoteConfig.tunnelName", mcpServerCatalogEntryIndex("spec.manifest.remoteConfig.tunnelName")).
			Build(),
	}
}

func mcpServerCatalogEntryIndex(field string) kclient.IndexerFunc {
	return func(object kclient.Object) []string {
		return []string{object.(*v1.MCPServerCatalogEntry).Get(field)}
	}
}

func callMCPTunnelHandler(
	t *testing.T,
	storage kclient.WithWatch,
	method, path, id, body string,
	handler api.HandlerFunc,
) (types.MCPTunnel, *httptest.ResponseRecorder, error) {
	t.Helper()

	var requestBody *strings.Reader
	if body == "" {
		requestBody = strings.NewReader("")
	} else {
		requestBody = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, requestBody)
	if id != "" {
		request.SetPathValue("id", id)
	}
	recorder := httptest.NewRecorder()
	err := handler(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
	})
	if err != nil {
		return types.MCPTunnel{}, recorder, err
	}

	var response types.MCPTunnel
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode MCP tunnel response: %v", err)
	}
	return response, recorder, nil
}

func callMCPTunnelListHandler(
	t *testing.T,
	storage kclient.WithWatch,
	handler api.HandlerFunc,
) (types.MCPTunnelList, error) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/api/mcp-tunnels", nil)
	recorder := httptest.NewRecorder()
	err := handler(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
	})
	if err != nil {
		return types.MCPTunnelList{}, err
	}

	var response types.MCPTunnelList
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode MCP tunnel list response: %v", err)
	}
	return response, nil
}

func decodeFullMCPTunnelToken(t *testing.T, token string) []byte {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	if len(raw) != 64 {
		t.Fatalf("token has %d raw bytes, want 64", len(raw))
	}
	return raw
}

func assertMCPTunnelPreview(t *testing.T, preview string, fullToken []byte) {
	t.Helper()

	encoded, ok := strings.CutSuffix(preview, "...")
	if !ok {
		t.Fatalf("preview = %q, want ellipsis suffix", preview)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("preview is not base64url: %v", err)
	}
	if len(raw) != 16 {
		t.Fatalf("preview has %d raw bytes, want 16", len(raw))
	}
	if !bytes.Equal(raw, fullToken[:16]) {
		t.Fatal("preview does not match the first 16 token bytes")
	}
}
