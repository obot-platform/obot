package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestDeleteAPIKeyEndpointRevokesAndRetainsTheKey(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	created, err := gatewayClient.CreateAPIKey(t.Context(), 7, "CLI token", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}

	userInfo := &user.DefaultInfo{UID: "7", Groups: []string{clienttypes.GroupAuthenticated}}
	ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodDelete, "/api/api-keys/"+strconv.FormatUint(uint64(created.ID), 10), nil, userInfo)
	ctx.SetPathValue("id", strconv.FormatUint(uint64(created.ID), 10))
	if err := s.revokeAPIKey(ctx); err != nil {
		t.Fatal(err)
	}

	var response map[string]bool
	decodeTokenRequestResponse(t, recorder, &response)
	if !response["deleted"] {
		t.Fatalf("response = %+v, want existing deleted=true contract", response)
	}

	retained, err := gatewayClient.GetAPIKey(t.Context(), 7, created.ID)
	if err != nil {
		t.Fatalf("get revoked API key: %v", err)
	}
	if retained.RevokedAt == nil {
		t.Fatalf("DELETE endpoint did not retain a revoked API key: %+v", retained)
	}
}

func TestAdminDeleteAPIKeyEndpointRevokesAndRetainsTheKey(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	created, err := gatewayClient.CreateAPIKey(t.Context(), 7, "CLI token", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}

	ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodDelete, "/api/admin-api-keys/"+strconv.FormatUint(uint64(created.ID), 10), nil, nil)
	ctx.SetPathValue("id", strconv.FormatUint(uint64(created.ID), 10))
	if err := s.deleteAnyAPIKey(ctx); err != nil {
		t.Fatal(err)
	}

	var response map[string]bool
	decodeTokenRequestResponse(t, recorder, &response)
	if !response["deleted"] {
		t.Fatalf("response = %+v, want existing deleted=true contract", response)
	}

	retained, err := gatewayClient.GetAPIKeyByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("get revoked API key: %v", err)
	}
	if retained.RevokedAt == nil {
		t.Fatalf("admin DELETE endpoint did not retain a revoked API key: %+v", retained)
	}
}

func TestAPIKeyListEndpointsHideRevokedKeysUnlessRequested(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	active, err := gatewayClient.CreateAPIKey(t.Context(), 7, "active", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := gatewayClient.CreateAPIKey(t.Context(), 7, "revoked", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gatewayClient.RevokeAPIKey(t.Context(), 7, revoked.ID); err != nil {
		t.Fatal(err)
	}

	userInfo := &user.DefaultInfo{UID: "7", Groups: []string{clienttypes.GroupAuthenticated}}
	for _, tt := range []struct {
		name         string
		path         string
		admin        bool
		wantKeyCount int
	}{
		{name: "user default", path: "/api/api-keys", wantKeyCount: 1},
		{name: "user show revoked", path: "/api/api-keys?show_revoked=true", wantKeyCount: 2},
		{name: "admin default", path: "/api/admin-api-keys", admin: true, wantKeyCount: 1},
		{name: "admin show revoked", path: "/api/admin-api-keys?show_revoked=true", admin: true, wantKeyCount: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodGet, tt.path, nil, userInfo)
			if tt.admin {
				err = s.listAllAPIKeys(ctx)
			} else {
				err = s.listAPIKeys(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Items []types.APIKey `json:"items"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Items) != tt.wantKeyCount {
				t.Fatalf("keys = %+v, want %d", response.Items, tt.wantKeyCount)
			}
			if response.Items[0].ID != active.ID && tt.wantKeyCount == 1 {
				t.Fatalf("default list returned revoked key: %+v", response.Items)
			}
		})
	}
}
