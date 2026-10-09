package handlers

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/stretchr/testify/require"
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
			name: "cannot expand allowlist",
			profiles: []gt.ClaudeEnvironmentProfile{{
				Name: "test",
				Subjects: []types.Subject{{
					Type: types.SubjectTypeSelector,
					ID:   "*",
				}},
				MCPServerIDs: []string{"outside"},
			}},
			errorText: "environment allowlist",
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
			gateway := newHandlerTestGateway(t)
			require.NoError(t, gateway.SaveClaudeEnvironment(t.Context(), &environment, nil))
			stored, err := gateway.ClaudeEnvironment(t.Context(), environment.ID)
			require.NoError(t, err)
			require.Equal(t, environment.Runtime, stored.Runtime)
			require.Equal(t, environment.Profiles, stored.Profiles)
		})
	}
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
