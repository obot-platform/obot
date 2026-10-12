package clauderunner

import (
	"testing"

	api "github.com/obot-platform/obot/apiclient/types"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestEnvironmentProfiles(t *testing.T) {
	user := &kuser.DefaultInfo{UID: "2", Extra: map[string][]string{"auth_provider_groups": {"engineering"}}}
	environment := gt.ClaudeEnvironment{
		MCPServerIDs: []string{"one", "two", "three"},
		Runtime:      gt.ClaudeRuntime{Resources: gt.ClaudeResources{ActorCPU: "1", ActorMemory: "2Gi", IdleTimeoutMinutes: 5, SessionRetentionDays: 30}},
		Profiles: []gt.ClaudeEnvironmentProfile{
			{Name: "user", Subjects: []api.Subject{{Type: api.SubjectTypeUser, ID: "2"}}, MCPServerIDs: []string{"one", "outside"}, Resources: gt.ClaudeResources{ActorCPU: "500m", ActorMemory: "3Gi"}},
			{Name: "team", Subjects: []api.Subject{{Type: api.SubjectTypeGroup, ID: "engineering"}}, MCPServerIDs: []string{"two"}, Resources: gt.ClaudeResources{ActorCPU: "1500m", IdleTimeoutMinutes: 10, SessionRetentionDays: -1}},
			{Name: "other", Subjects: []api.Subject{{Type: api.SubjectTypeUser, ID: "3"}}, Resources: gt.ClaudeResources{ActorCPU: "10"}},
		},
	}
	resolved, err := ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, resolved.MCPServerIDs)
	require.Equal(t, gt.ClaudeResources{ActorCPU: "1500m", ActorMemory: "3Gi", IdleTimeoutMinutes: 10, SessionRetentionDays: -1}, resolved.Runtime.Resources)
	environment.Profiles = environment.Profiles[:1]
	environment.Profiles[0].MCPServerIDs = nil
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.ElementsMatch(t, environment.MCPServerIDs, resolved.MCPServerIDs)
	require.Equal(t, "500m", resolved.Runtime.Resources.ActorCPU, "profiles may lower the environment default")
	environment.Profiles[0].MCPServerIDs = []string{}
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Empty(t, resolved.MCPServerIDs)
	environment.Profiles = nil
	_, err = ResolveEnvironment(user, &environment)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	manifest := api.VMCPManifest{DisplayName: "environment"}
	manifest.Default(false)
	environment.Profiles = []gt.ClaudeEnvironmentProfile{{Name: manifest.Profiles[0].Name, Subjects: manifest.Profiles[0].Subjects}}
	_, err = ResolveEnvironment(user, &environment)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = ResolveEnvironment(&kuser.DefaultInfo{Extra: map[string][]string{"obot_groups": {api.GroupAdmin}}}, &environment)
	require.NoError(t, err)
}

func TestProfileEgressAddsAccess(t *testing.T) {
	user := &kuser.DefaultInfo{UID: "2"}
	environment := gt.ClaudeEnvironment{
		Runtime: gt.ClaudeRuntime{EgressHosts: []string{"base.example.com"}},
		Profiles: []gt.ClaudeEnvironmentProfile{
			{
				Name:        "first",
				Subjects:    []api.Subject{{Type: api.SubjectTypeUser, ID: "2"}},
				EgressHosts: []string{"one.example.com", "base.example.com"},
			},
			{
				Name:        "second",
				Subjects:    []api.Subject{{Type: api.SubjectTypeUser, ID: "2"}},
				EgressHosts: []string{"two.example.com", "one.example.com"},
			},
			{
				Name:           "other",
				Subjects:       []api.Subject{{Type: api.SubjectTypeUser, ID: "3"}},
				AllowAllEgress: true,
				EgressHosts:    []string{"other.example.com"},
			},
		},
	}
	resolved, err := ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Equal(t, []string{"base.example.com", "one.example.com", "two.example.com"}, resolved.Runtime.EgressHosts)
	require.False(t, resolved.Runtime.AllowAllEgress)
	require.Equal(t, []string{"base.example.com"}, environment.Runtime.EgressHosts)

	environment.Profiles[0].AllowAllEgress = true
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.True(t, resolved.Runtime.AllowAllEgress)
	require.False(t, environment.Runtime.AllowAllEgress)

	environment.Profiles[0].AllowAllEgress = false
	environment.Profiles[0].EgressHosts = nil
	environment.Profiles[1].EgressHosts = []string{}
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Equal(t, environment.Runtime.EgressHosts, resolved.Runtime.EgressHosts)
	require.False(t, resolved.Runtime.AllowAllEgress)

	environment.Runtime.AllowAllEgress = true
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.True(t, resolved.Runtime.AllowAllEgress)
}

func TestEgressSecretProfilePrecedence(t *testing.T) {
	user := &kuser.DefaultInfo{UID: "2"}
	environment := gt.ClaudeEnvironment{
		EgressSecrets: []gt.ClaudeEgressSecret{{ID: "env", Hostname: "api.example.com", Header: "authorization", Value: "env"}},
		Profiles: []gt.ClaudeEnvironmentProfile{
			{Name: "other", Subjects: []api.Subject{{Type: api.SubjectTypeUser, ID: "3"}}, EgressSecrets: []gt.ClaudeEgressSecret{{ID: "other", Hostname: "api.example.com", Header: "authorization", Value: "other"}}},
			{Name: "first", Subjects: []api.Subject{{Type: api.SubjectTypeUser, ID: "2"}}, EgressSecrets: []gt.ClaudeEgressSecret{{ID: "first", Hostname: "api.example.com", Header: "Authorization", Value: "first"}}},
			{Name: "second", Subjects: []api.Subject{{Type: api.SubjectTypeUser, ID: "2"}}, EgressSecrets: []gt.ClaudeEgressSecret{{ID: "second", Hostname: "api.example.com", Header: "authorization", Value: "second"}}},
		},
	}
	resolved, err := ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Len(t, resolved.EgressSecrets, 1)
	require.Equal(t, "first", resolved.EgressSecrets[0].Value)
	require.Equal(t, "env", environment.EgressSecrets[0].Value)
	environment.Profiles = environment.Profiles[:1]
	environment.Profiles[0].Subjects[0].ID = "2"
	environment.Profiles[0].EgressSecrets = nil
	resolved, err = ResolveEnvironment(user, &environment)
	require.NoError(t, err)
	require.Equal(t, "env", resolved.EgressSecrets[0].Value)
}
