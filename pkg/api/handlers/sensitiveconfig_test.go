package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	sensitiveConfigValue = "sensitive-inline-value"
	plainConfigValue     = "plain-inline-value"
)

func sensitiveConfigForTest() []types.MCPConfig {
	return []types.MCPConfig{
		{
			Key:       "API_TOKEN",
			Value:     sensitiveConfigValue,
			Sensitive: true,
		},
		{
			Key:   "REGION",
			Value: plainConfigValue,
		},
	}
}

func sensitiveSystemServerManifestForTest() types.SystemMCPServerManifest {
	return types.SystemMCPServerManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{
			URL: "https://example.com/mcp",
			DeprecatedHeaders: []types.MCPHeader{{ //nolint:staticcheck // Legacy values are still returned when stored.
				Key:       "Authorization",
				Value:     sensitiveConfigValue,
				Sensitive: true,
			}},
		},
		Config: sensitiveConfigForTest(),
		DeprecatedEnv: []types.MCPEnv{{ //nolint:staticcheck // Legacy values are still returned when stored.
			Key:       "LEGACY_TOKEN",
			Value:     sensitiveConfigValue,
			Sensitive: true,
		}},
	}
}

// TestSensitiveInlineConfigIsRedactedForReaders checks that list and get responses for system servers, webhook
// validations and system catalog entries carry inline sensitive values to admins and auditors only.
func TestSensitiveInlineConfigIsRedactedForReaders(t *testing.T) {
	const (
		serverName  = system.SystemMCPServerPrefix + "sensitive"
		webhookName = "mwhv1sensitive"
		catalogName = "syscat-sensitive"
		entryName   = "syscat-sensitive-entry"
	)
	server := &v1.SystemMCPServer{
		Name:      serverName,
		Namespace: system.DefaultNamespace,
		Spec:      v1.SystemMCPServerSpec{Manifest: sensitiveSystemServerManifestForTest()},
	}
	webhookManifest := sensitiveSystemServerManifestForTest()
	webhook := &v1.MCPWebhookValidation{
		Name:      webhookName,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			SystemMCPServerManifest: &webhookManifest,
		}},
	}
	catalog := &v1.SystemMCPCatalog{
		Name:      catalogName,
		Namespace: system.DefaultNamespace,
	}
	entry := &v1.SystemMCPServerCatalogEntry{
		Name:      entryName,
		Namespace: system.DefaultNamespace,
		Spec: v1.SystemMCPServerCatalogEntrySpec{
			SystemMCPCatalogName: catalogName,
			Manifest: types.SystemMCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Config:  sensitiveConfigForTest(),
			},
		},
	}
	_, storage, gateway := newVMCPActionSessionManager(t, server, webhook, catalog, entry)

	systemHandler := &SystemMCPServerHandler{}
	webhookHandler := &MCPWebhookValidationHandler{}
	catalogHandler := &SystemMCPCatalogHandler{}
	endpoints := []struct {
		name    string
		handler func(api.Context) error
		// sensitive is how many sensitive values the endpoint returns to an admin.
		sensitive int
	}{
		{
			name:      "list system servers",
			handler:   systemHandler.List,
			sensitive: 3,
		},
		{
			name:      "get system server",
			handler:   systemHandler.Get,
			sensitive: 3,
		},
		{
			name:      "list webhook validations",
			handler:   webhookHandler.List,
			sensitive: 3,
		},
		{
			name:      "get webhook validation",
			handler:   webhookHandler.Get,
			sensitive: 3,
		},
		{
			name:      "get system catalog entry",
			handler:   catalogHandler.GetEntry,
			sensitive: 1,
		},
	}
	roles := []struct {
		name     string
		role     types.Role
		redacted bool
	}{
		{
			name: "admin",
			role: types.RoleAdmin,
		},
		{
			name:     "admin read-only",
			role:     types.RoleBasic | types.RoleAdminReadonly,
			redacted: true,
		},
		{
			name: "admin read-only with auditor",
			role: types.RoleBasic | types.RoleAdminReadonly | types.RoleAuditor,
		},
		{
			name: "auditor",
			role: types.RoleBasic | types.RoleAuditor,
		},
	}

	for _, role := range roles {
		for _, endpoint := range endpoints {
			t.Run(role.name+"/"+endpoint.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.SetPathValue("id", serverName)
				request.SetPathValue("mcp_webhook_validation_id", webhookName)
				request.SetPathValue("catalog_id", catalogName)
				request.SetPathValue("entry_id", entryName)
				recorder := httptest.NewRecorder()
				require.NoError(t, endpoint.handler(api.Context{
					Request:        request,
					ResponseWriter: recorder,
					Storage:        storage,
					GatewayClient:  gateway,
					User: &kuser.DefaultInfo{
						UID:    "reader",
						Groups: role.role.Groups(),
					},
				}))

				body := recorder.Body.String()
				require.True(t, json.Valid([]byte(body)), body)
				require.Contains(t, body, plainConfigValue, "non-sensitive values are not redacted")
				shown, masked := endpoint.sensitive, 0
				if role.redacted {
					shown, masked = 0, endpoint.sensitive
				}
				require.Equal(t, shown, strings.Count(body, sensitiveConfigValue), body)
				require.Equal(t, masked, strings.Count(body, `"`+redactedValue+`"`), body)
			})
		}
	}

	// Redaction works on copies; the stored objects keep their values.
	var stored v1.SystemMCPServer
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: serverName}, &stored))
	require.Equal(t, sensitiveConfigValue, stored.Spec.Manifest.Config[0].Value)
}

func TestRedactSensitiveConfigLeavesUnsetValuesEmpty(t *testing.T) {
	config := []types.MCPConfig{
		{
			Key:       "UNSET_TOKEN",
			Sensitive: true,
		},
		{
			Key:       "API_TOKEN",
			Value:     sensitiveConfigValue,
			Sensitive: true,
		},
	}
	redacted := redactSensitiveConfig(config)

	require.Empty(t, redacted[0].Value, "an unset value is not shown as hidden")
	require.Equal(t, redactedValue, redacted[1].Value)
}

func TestRedactSystemMCPServerManifestLeavesInputUnchanged(t *testing.T) {
	manifest := sensitiveSystemServerManifestForTest()
	redacted := redactSystemMCPServerManifest(manifest)

	require.Equal(t, redactedValue, redacted.Config[0].Value)
	require.Equal(t, plainConfigValue, redacted.Config[1].Value)
	require.Equal(t, redactedValue, redacted.DeprecatedEnv[0].Value)                  //nolint:staticcheck // Legacy values are still returned when stored.
	require.Equal(t, redactedValue, redacted.RemoteConfig.DeprecatedHeaders[0].Value) //nolint:staticcheck // Legacy values are still returned when stored.

	require.Equal(t, sensitiveConfigValue, manifest.Config[0].Value)
	require.Equal(t, sensitiveConfigValue, manifest.DeprecatedEnv[0].Value)                  //nolint:staticcheck // Legacy values are still returned when stored.
	require.Equal(t, sensitiveConfigValue, manifest.RemoteConfig.DeprecatedHeaders[0].Value) //nolint:staticcheck // Legacy values are still returned when stored.
}

func TestReadOnlyAdminMasksOtherUsersCapturedFiles(t *testing.T) {
	gateway := newEnforcementTestGatewayClient(t)
	scan := &gtypes.DeviceScan{
		SubmittedBy: "42",
		Files: []gtypes.DeviceScanFile{
			{
				Path:    "/example/config.json",
				Content: sensitiveConfigValue,
			},
		},
	}
	require.NoError(t, gateway.InsertDeviceScan(t.Context(), scan))

	for _, tt := range []struct {
		name        string
		userID      string
		role        types.Role
		wantContent string
	}{
		{
			name:   "other user without Auditor",
			userID: "7",
			role:   types.RoleBasic | types.RoleAdminReadonly,
		},
		{
			name:        "own scan",
			userID:      "42",
			role:        types.RoleBasic | types.RoleAdminReadonly,
			wantContent: sensitiveConfigValue,
		},
		{
			name:        "other user with Auditor",
			userID:      "7",
			role:        types.RoleBasic | types.RoleAdminReadonly | types.RoleAuditor,
			wantContent: sensitiveConfigValue,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/devices/scans", nil)
			request.SetPathValue("scan_id", strconv.FormatUint(uint64(scan.ID), 10))
			recorder := httptest.NewRecorder()
			require.NoError(t, (&DeviceScansHandler{}).Get(api.Context{
				Request:        request,
				ResponseWriter: recorder,
				GatewayClient:  gateway,
				User: &kuser.DefaultInfo{
					UID:    tt.userID,
					Groups: tt.role.Groups(),
				},
			}))
			var result types.DeviceScan
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			require.Len(t, result.Files, 1)
			require.Equal(t, scan.Files[0].Path, result.Files[0].Path)
			require.Equal(t, tt.wantContent, result.Files[0].Content)
		})
	}
}
