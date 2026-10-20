package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestEnvironmentProfileValidation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		profiles     []gt.ClaudeEnvironmentProfile
		wantProfiles int
		errorText    string
	}{
		{
			name:         "default admin",
			wantProfiles: 1,
		},
		{
			name:         "explicit deny all",
			profiles:     []gt.ClaudeEnvironmentProfile{},
			wantProfiles: 0,
		},
		{
			name: "actor resources must be positive",
			profiles: []gt.ClaudeEnvironmentProfile{{
				Name: "test",
				Subjects: []types.Subject{{
					Type: types.SubjectTypeSelector,
					ID:   "*",
				}},
				Resources: gt.ClaudeResources{ActorMemory: "0"},
			}},
			errorText: "positive",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment := gt.ClaudeEnvironment{
				ID:            "test",
				DisplayName:   "Test",
				EnvironmentID: "ccpool_test",
				Profiles:      tc.profiles,
				Runtime:       gt.ClaudeRuntime{Image: "registry/runner@sha256:" + strings.Repeat("a", 64)},
			}
			h := ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{WorkerImage: "worker:test", SnapshotLocation: "s3://bucket"}}}
			err := h.validateEnvironment(api.Context{}, &environment, true)
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				return
			}
			require.NoError(t, err)
			require.Len(t, environment.Profiles, tc.wantProfiles)
			require.Equal(t, "1", environment.Runtime.Resources.ActorCPU)
			require.Equal(t, "2Gi", environment.Runtime.Resources.ActorMemory)
			gateway, db := newHandlerTestGatewayWithDB(t)
			require.NoError(t, gateway.SaveClaudeEnvironment(t.Context(), &environment, nil))
			stored, err := gateway.ClaudeEnvironment(t.Context(), environment.ID)
			require.NoError(t, err)
			require.Equal(t, environment.Runtime, stored.Runtime)
			require.Equal(t, environment.Profiles, stored.Profiles)

			// A cleaned-up key must not prevent refreshing the following session.
			var instances []kclient.Object
			var activeKey gt.APIKey
			for _, name := range []string{"a-expired", "z-active"} {
				key := gt.APIKey{
					UserID:            123,
					ClaudeEnvironment: true,
					ExpiresAt:         new(time.Now().Add(time.Hour)),
				}
				require.NoError(t, db.Create(&key).Error)
				require.NoError(t, gateway.SaveClaudeWorkOrder(t.Context(), gt.Credential{
					Context: "substrate-agent",
					Name:    name,
					Secrets: map[string]string{"keyID": fmt.Sprint(key.ID)},
				}))
				instances = append(instances, &v1.AgentInstance{
					Name:      name,
					Namespace: "default",
					Spec: v1.AgentInstanceSpec{
						ClaudeEnvironment: environment.ID,
						ClaudeAccount:     "removed-account",
						UserID:            "123",
					},
				})
				if name == "a-expired" {
					require.NoError(t, db.Delete(&key).Error)
				} else {
					activeKey = key
				}
			}
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(instances...).Build()
			require.NoError(t, refreshEnvironmentCredentials(api.Context{
				Request:       httptest.NewRequest(http.MethodPut, "/", nil),
				GatewayClient: gateway,
				Storage:       storage,
			}, &environment))
			refreshed, err := gateway.GetAPIKeyByID(t.Context(), activeKey.ID)
			require.NoError(t, err)
			require.NotNil(t, refreshed.RevokedAt)
		})
	}
}

func TestEnvironmentIgnoresWorkerSchedulingOverrides(t *testing.T) {
	var environment gt.ClaudeEnvironment
	require.NoError(t, json.Unmarshal([]byte(`{"runtime":{"worker":{"nodeSelector":{"workload":"custom"},"tolerations":[{"operator":"Exists"}]}}}`), &environment))
	encoded, err := json.Marshal(environment)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "nodeSelector")
	require.NotContains(t, string(encoded), "tolerations")
}

func TestProfileVMCPValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ids       []string
		owner     string
		errorText string
	}{
		{
			name: "no vMCPs",
		},
		{
			name: "shared vMCP",
			ids:  []string{"server"},
		},
		{
			name:      "missing vMCP",
			ids:       []string{"missing"},
			errorText: "existing vMCPs",
		},
		{
			name:      "personal vMCP",
			ids:       []string{"server"},
			owner:     "user",
			errorText: "shared vMCPs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &v1.VMCP{}
			server.Name = "server"
			server.Namespace = "default"
			server.Spec.UserID = tc.owner
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server).Build()
			environment := gt.ClaudeEnvironment{
				DisplayName: "Test",
				Profiles: []gt.ClaudeEnvironmentProfile{{
					Name:         "test",
					Subjects:     []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					MCPServerIDs: tc.ids,
				}},
				Runtime: gt.ClaudeRuntime{Image: "registry/runner@sha256:" + strings.Repeat("a", 64)},
			}
			h := ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{WorkerImage: "worker:test"}}}
			err := h.validateEnvironment(api.Context{
				Request: httptest.NewRequest(http.MethodPost, "/api/claude-environments", nil),
				Storage: storage,
			}, &environment, true)
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestProfileVMCPValidationFetchesEachServerOncePerSave(t *testing.T) {
	lookups := map[string]int{}
	builder := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
				lookups[key.Name]++
				return client.Get(ctx, key, obj, opts...)
			},
		})
	var ids []string
	for i := range 32 {
		server := &v1.VMCP{}
		server.Name = fmt.Sprintf("server-%d", i)
		server.Namespace = "default"
		builder.WithObjects(server)
		ids = append(ids, server.Name)
	}

	environment := gt.ClaudeEnvironment{
		DisplayName: "Test",
		Runtime:     gt.ClaudeRuntime{Image: "registry/runner@sha256:" + strings.Repeat("a", 64)},
	}
	for i := range 100 {
		environment.Profiles = append(environment.Profiles, gt.ClaudeEnvironmentProfile{
			Name:         fmt.Sprintf("profile-%d", i),
			Subjects:     []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			MCPServerIDs: ids,
		})
	}
	request := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/claude-environments", nil),
		Storage: builder.Build(),
	}
	h := ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{WorkerImage: "worker:test"}}}

	for save := range 2 {
		require.NoError(t, h.validateEnvironment(request, &environment, true))
		require.Len(t, lookups, len(ids))
		for _, id := range ids {
			require.Equal(t, save+1, lookups[id], "vMCP %s must be fetched once per save", id)
		}
	}
}

func TestEnvironmentIgnoresLegacyVMCPs(t *testing.T) {
	var environment gt.ClaudeEnvironment
	require.NoError(t, json.Unmarshal([]byte(`{"mcpServerIDs":["legacy"],"profiles":[{"mcpServerIDs":["profile"]}]}`), &environment))
	encoded, err := json.Marshal(environment)
	require.NoError(t, err)

	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &result))
	require.NotContains(t, result, "mcpServerIDs")
	require.Equal(t, []string{"profile"}, environment.Profiles[0].MCPServerIDs)
}

func TestEnvironmentKeyIdentity(t *testing.T) {
	const organization = "84c0eb0d-afdc-4d78-9635-a51b1f1d4ed4"
	key := func(payload string) string {
		return "sk-ant-api03-" + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString([]byte("signature"))
	}
	valid := key(`{"organization_uuid":"` + organization + `","ccr:pool_id":"ccpool_test"}`)

	var environment gt.ClaudeEnvironment
	require.Error(t, applyClaudeEnvironmentKey(&environment, ""))
	require.NoError(t, applyClaudeEnvironmentKey(&environment, " "+valid+"\n"))
	require.Equal(t, organization, environment.OrganizationID)
	require.Equal(t, "ccpool_test", environment.EnvironmentID)
	require.NoError(t, applyClaudeEnvironmentKey(&environment, ""))
	require.NoError(t, applyClaudeEnvironmentKey(&environment, valid))

	for _, invalid := range []string{
		"not-a-key",
		strings.Repeat("a", 32769),
		key(`{"ccr:pool_id":"ccpool_test"}`),
		key(`{"organization_uuid":"invalid","ccr:pool_id":"ccpool_test"}`),
		key(`{"organization_uuid":"` + organization + `","ccr:pool_id":"invalid"}`),
		key(`{"organization_uuid":"` + organization + `","ccr:pool_id":"ccpool_other"}`),
		key(`{"organization_uuid":"00000000-0000-0000-0000-000000000001","ccr:pool_id":"ccpool_test"}`),
	} {
		before := environment
		require.Error(t, applyClaudeEnvironmentKey(&environment, invalid))
		require.Equal(t, before, environment)
	}
}

func TestProfileEgressValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hosts     []string
		wantError bool
	}{
		{
			name:  "additional host outside environment list",
			hosts: []string{"*.github.com"},
		},
		{
			name:      "invalid hostname",
			hosts:     []string{"https://example.com"},
			wantError: true,
		},
		{
			name:      "combined host limit",
			hosts:     []string{"extra.example.com"},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment := gt.ClaudeEnvironment{
				DisplayName: "Test",
				Runtime:     gt.ClaudeRuntime{Image: "registry/runner@sha256:" + strings.Repeat("a", 64)},
				Profiles: []gt.ClaudeEnvironmentProfile{{
					Name:           "test",
					Subjects:       []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					EgressHosts:    tc.hosts,
					AllowAllEgress: true,
				}},
			}
			if tc.name == "combined host limit" {
				for i := range 100 {
					environment.Runtime.EgressHosts = append(environment.Runtime.EgressHosts, fmt.Sprintf("host%d.example.com", i))
				}
			}
			h := ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{WorkerImage: "worker:test"}}}
			err := h.validateEnvironment(api.Context{}, &environment, true)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
