package vmcp

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	profileExplicit = "vp1explicit"
	profileOther    = "vp1other"
	profileAgents   = "vp1agents"
	profileUsers    = "vp1users"
)

// memoryCredentials is a credential store in memory, keyed by context and then name.
type memoryCredentials struct {
	lock        sync.Mutex
	credentials map[string]map[string]map[string]string
	// onUpsert runs before a credential is stored.
	onUpsert func(gatewaytypes.Credential)
	// contextQueries counts the calls to CredentialContextsWithCredentials.
	contextQueries int
}

func newMemoryCredentials() *memoryCredentials {
	return &memoryCredentials{credentials: map[string]map[string]map[string]string{}}
}

func (m *memoryCredentials) RevealCredential(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, credentialContext := range contexts {
		if secrets, ok := m.credentials[credentialContext][name]; ok {
			return gatewaytypes.Credential{Context: credentialContext, Name: name, Secrets: maps.Clone(secrets)}, nil
		}
	}
	return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{Contexts: contexts, Name: name}
}

func (m *memoryCredentials) UpsertCredential(_ context.Context, credential gatewaytypes.Credential) error {
	if m.onUpsert != nil {
		m.onUpsert(credential)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.credentials[credential.Context] == nil {
		m.credentials[credential.Context] = map[string]map[string]string{}
	}
	m.credentials[credential.Context][credential.Name] = maps.Clone(credential.Secrets)
	return nil
}

func (m *memoryCredentials) DeleteCredential(_ context.Context, credentialContext, name string) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.credentials[credentialContext][name]
	delete(m.credentials[credentialContext], name)
	if len(m.credentials[credentialContext]) == 0 {
		delete(m.credentials, credentialContext)
	}
	return ok, nil
}

func (m *memoryCredentials) CredentialContextsWithCredentials(_ context.Context, contexts []string) ([]string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.contextQueries++
	var found []string
	for _, credentialContext := range contexts {
		if len(m.credentials[credentialContext]) > 0 {
			found = append(found, credentialContext)
		}
	}
	return found, nil
}

func (m *memoryCredentials) DeleteCredentialsInContexts(_ context.Context, contexts []string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, credentialContext := range contexts {
		delete(m.credentials, credentialContext)
	}
	return nil
}

// revisions returns the names of the revisions stored for a profile.
func (m *memoryCredentials) revisions(vmcpID, profileID string) []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	return slices.Sorted(maps.Keys(m.credentials[ProfileConfigurationCredentialContext(vmcpID, profileID)]))
}

// profileConfiguredComponent returns a component with a required and an optional value a user would enter, a value
// the vMCP fixes, a value its catalog entry sets, one it binds to a Secret, and one the vMCP prohibits.
func profileConfiguredComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Config = []types.MCPConfig{
		{Key: "REGION", Required: true},
		{Key: "VERBOSE"},
		{Key: "ADMIN_TOKEN", Required: true},
		{Key: "CATALOG_LITERAL", Static: true, Value: "catalog"},
		{Key: "CATALOG_SECRET", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "key"}},
		{Key: "BLOCKED"},
	}
	component.Configuration = []types.VMCPConfigurationPolicy{
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
		{Key: "CATALOG_LITERAL", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "CATALOG_SECRET", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "BLOCKED", Policy: types.VMCPConfigurationPolicyProhibited},
	}
	return component
}

// hostnameComponent returns a remote component whose URL each connection enters, on a required hostname.
func hostnameComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{Hostname: "tenant.example.com"}
	return component
}

func everyAgentProfile(id string, subject types.SubjectType) types.VMCPProfile {
	return types.VMCPProfile{
		ID:          id,
		Name:        id,
		Subjects:    []types.Subject{{Type: subject, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
}

func explicitAgentProfile(id, agentID string) types.VMCPProfile {
	return types.VMCPProfile{
		ID:          id,
		Name:        id,
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: agentID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
}

// profileRevisions returns the status of profiles whose current revisions set keys, by profile ID; each revision is
// named after its profile.
func profileRevisions(componentID string, keys map[string][]string) map[string]v1.VMCPProfileConfigurationRevision {
	result := map[string]v1.VMCPProfileConfigurationRevision{}
	for profileID, fields := range keys {
		revision := v1.VMCPProfileConfigurationRevision{Name: "configuration-" + profileID}
		for _, field := range fields {
			revision.Keys = append(revision.Keys, ConfigurationKey(componentID, field))
		}
		result[profileID] = revision
	}
	return result
}

func source(profileID, field string, fallback bool) ConfigurationSource {
	return ConfigurationSource{ComponentID: "api", Field: field, ProfileID: profileID, Fallback: fallback, Revision: "configuration-" + profileID}
}

// TestResolveComponentConfiguration is the precedence table of profile configuration: the explicit profile first, the
// wildcard only for required values it does not set, and optional values never falling back.
func TestResolveComponentConfiguration(t *testing.T) {
	component := profileConfiguredComponent("api")
	for _, tt := range []struct {
		name        string
		profiles    []types.VMCPProfile
		keys        map[string][]string
		wantSources []ConfigurationSource
		wantMissing []string
		wantAmbig   bool
	}{
		{
			name:     "explicit profile only",
			profiles: []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7")},
			keys:     map[string][]string{profileExplicit: {"REGION", "VERBOSE"}},
			wantSources: []ConfigurationSource{
				source(profileExplicit, "REGION", false),
				source(profileExplicit, "VERBOSE", false),
			},
		},
		{
			// With no explicit profile, the wildcard's configuration is used as it is, optional values included.
			name:     "wildcard profile only",
			profiles: []types.VMCPProfile{everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:     map[string][]string{profileAgents: {"REGION", "VERBOSE"}},
			wantSources: []ConfigurationSource{
				source(profileAgents, "REGION", true),
				source(profileAgents, "VERBOSE", true),
			},
		},
		{
			// The explicit profile wins, and its unset optional value does not fall back.
			name:        "both profiles",
			profiles:    []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:        map[string][]string{profileExplicit: {"REGION"}, profileAgents: {"REGION", "VERBOSE"}},
			wantSources: []ConfigurationSource{source(profileExplicit, "REGION", false)},
		},
		{
			name:     "required value falls back",
			profiles: []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeSelector)},
			keys:     map[string][]string{profileExplicit: {"VERBOSE"}, profileAgents: {"REGION"}},
			wantSources: []ConfigurationSource{
				source(profileAgents, "REGION", true),
				source(profileExplicit, "VERBOSE", false),
			},
		},
		{
			// A value cleared from the explicit profile is one it does not set.
			name:        "explicit profile without configuration",
			profiles:    []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:        map[string][]string{profileAgents: {"REGION", "VERBOSE"}},
			wantSources: []ConfigurationSource{source(profileAgents, "REGION", true)},
		},
		{
			name:        "required value set nowhere",
			profiles:    []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:        map[string][]string{profileExplicit: {"VERBOSE"}},
			wantSources: []ConfigurationSource{source(profileExplicit, "VERBOSE", false)},
			wantMissing: []string{"REGION"},
		},
		{
			// Removing the agent from its explicit profile makes it fall back for every value; it revokes nothing.
			name:     "explicit assignment removed",
			profiles: []types.VMCPProfile{explicitAgentProfile(profileExplicit, "8"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:     map[string][]string{profileExplicit: {"REGION", "VERBOSE"}, profileAgents: {"REGION", "VERBOSE"}},
			wantSources: []ConfigurationSource{
				source(profileAgents, "REGION", true),
				source(profileAgents, "VERBOSE", true),
			},
		},
		{
			// Another agent's explicit profile never supplies this agent.
			name:        "another agent's profile",
			profiles:    []types.VMCPProfile{explicitAgentProfile(profileOther, "8"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:        map[string][]string{profileOther: {"REGION", "VERBOSE"}},
			wantMissing: []string{"REGION"},
		},
		{
			// A key the vMCP now fixes, or that the catalog entry sets, is never taken from a profile, even one whose
			// revision predates the change.
			name:        "keys a profile cannot set",
			profiles:    []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7")},
			keys:        map[string][]string{profileExplicit: {"ADMIN_TOKEN", "CATALOG_LITERAL", "CATALOG_SECRET", "BLOCKED"}},
			wantMissing: []string{"REGION"},
		},
		{
			name: "ambiguous fallback for a required value",
			profiles: []types.VMCPProfile{
				explicitAgentProfile(profileExplicit, "7"),
				everyAgentProfile(profileAgents, types.SubjectTypeSelector),
				everyAgentProfile(profileUsers, types.SubjectTypeSelector),
			},
			keys:        map[string][]string{profileExplicit: {"VERBOSE"}},
			wantSources: []ConfigurationSource{source(profileExplicit, "VERBOSE", false)},
			wantAmbig:   true,
		},
		{
			// Optional values never fall back, so they are never ambiguous.
			name: "ambiguous fallback not needed",
			profiles: []types.VMCPProfile{
				explicitAgentProfile(profileExplicit, "7"),
				everyAgentProfile(profileAgents, types.SubjectTypeSelector),
				everyAgentProfile(profileUsers, types.SubjectTypeSelector),
			},
			keys:        map[string][]string{profileExplicit: {"REGION"}},
			wantSources: []ConfigurationSource{source(profileExplicit, "REGION", false)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selection := SelectAgentProfiles(agentInfo(7, nil), types.VMCPManifest{Profiles: tt.profiles})
			got := ResolveComponentConfiguration(component, selection, profileRevisions("api", tt.keys))
			require.Equal(t, "api", got.ComponentID)
			require.Equal(t, tt.wantSources, got.Sources)
			require.Equal(t, tt.wantMissing, got.Missing)
			require.Equal(t, tt.wantAmbig, got.AmbiguousFallback)
		})
	}
}

// TestProfileConfigurationReadiness checks the default configuration check of agent readiness, from the keys that
// profiles publish.
func TestProfileConfigurationReadiness(t *testing.T) {
	for _, tt := range []struct {
		name        string
		agent       uint
		components  []types.VMCPComponent
		profiles    []types.VMCPProfile
		keys        map[string][]string
		wantReasons []ReadinessReason
	}{
		{
			name:       "explicit profile supplies every required value",
			agent:      7,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles:   []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:       map[string][]string{profileExplicit: {"REGION"}},
		},
		{
			name:       "required value supplied by the fallback",
			agent:      7,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles:   []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:       map[string][]string{profileAgents: {"REGION"}},
		},
		{
			name:       "missing value is fixed in the explicit profile",
			agent:      7,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles:   []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:       map[string][]string{profileAgents: {"VERBOSE"}},
			wantReasons: []ReadinessReason{{
				ComponentID: "api",
				Code:        ReadinessConfigurationRequired,
				Field:       "REGION",
				ProfileID:   profileExplicit,
			}},
		},
		{
			name:       "missing value is fixed in the wildcard profile",
			agent:      9,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles:   []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7"), everyAgentProfile(profileAgents, types.SubjectTypeAgent)},
			keys:       map[string][]string{profileExplicit: {"REGION"}},
			wantReasons: []ReadinessReason{{
				ComponentID: "api",
				Code:        ReadinessConfigurationRequired,
				Field:       "REGION",
				ProfileID:   profileAgents,
			}},
		},
		{
			name:       "hostname-constrained URL",
			agent:      7,
			components: []types.VMCPComponent{hostnameComponent("remote")},
			profiles:   []types.VMCPProfile{explicitAgentProfile(profileExplicit, "7")},
			wantReasons: []ReadinessReason{{
				ComponentID: "remote",
				Code:        ReadinessConfigurationRequired,
				Field:       "__url",
				ProfileID:   profileExplicit,
			}},
		},
		{
			name:       "explicit profile supplies everything on an ambiguous vMCP",
			agent:      7,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles: []types.VMCPProfile{
				explicitAgentProfile(profileExplicit, "7"),
				everyAgentProfile(profileAgents, types.SubjectTypeSelector),
				everyAgentProfile(profileUsers, types.SubjectTypeSelector),
			},
			keys: map[string][]string{profileExplicit: {"REGION"}},
		},
		{
			name:       "ambiguous fallback",
			agent:      7,
			components: []types.VMCPComponent{profileConfiguredComponent("api")},
			profiles: []types.VMCPProfile{
				explicitAgentProfile(profileExplicit, "7"),
				everyAgentProfile(profileAgents, types.SubjectTypeSelector),
				everyAgentProfile(profileUsers, types.SubjectTypeSelector),
			},
			wantReasons: []ReadinessReason{AmbiguousFallbackReason("api")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := readinessVMCP(tt.components, tt.profiles...)
			vmcp.Status.ProfileConfigurationRevisions = profileRevisions(tt.components[0].ID, tt.keys)
			readiness, err := AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, agentInfo(tt.agent, nil))
			require.NoError(t, err)
			require.Equal(t, tt.wantReasons, readiness.Reasons)
		})
	}
}

func TestValidateProfileConfiguration(t *testing.T) {
	components := ComponentsForInstance(readinessVMCP([]types.VMCPComponent{profileConfiguredComponent("api"), hostnameComponent("remote")}), v1.VMCPInstance{})
	for _, tt := range []struct {
		name        string
		update      map[string]map[string]*string
		wantSet     map[string]string
		wantCleared []string
		wantErr     string
	}{
		{
			name: "values a user would enter",
			update: map[string]map[string]*string{
				"api":    {"REGION": new(" us-east "), "VERBOSE": nil},
				"remote": {"__url": new("https://tenant.example.com/mcp")},
			},
			wantSet: map[string]string{
				ConfigurationKey("api", "REGION"):   "us-east",
				ConfigurationKey("remote", "__url"): "https://tenant.example.com/mcp",
			},
			wantCleared: []string{ConfigurationKey("api", "VERBOSE")},
		},
		{
			name:        "blank value clears",
			update:      map[string]map[string]*string{"api": {"REGION": new("  ")}},
			wantSet:     map[string]string{},
			wantCleared: []string{ConfigurationKey("api", "REGION")},
		},
		{
			name:    "fixed value",
			update:  map[string]map[string]*string{"api": {"ADMIN_TOKEN": new("override")}},
			wantErr: "fixed by the vMCP",
		},
		{
			// Clearing is a write too, so a fixed key cannot be cleared either.
			name:    "clearing a fixed value",
			update:  map[string]map[string]*string{"api": {"ADMIN_TOKEN": nil}},
			wantErr: "fixed by the vMCP",
		},
		{
			name:    "catalog-static value",
			update:  map[string]map[string]*string{"api": {"CATALOG_LITERAL": new("override")}},
			wantErr: "set by its catalog entry",
		},
		{
			name:    "Secret-bound value",
			update:  map[string]map[string]*string{"api": {"CATALOG_SECRET": new("override")}},
			wantErr: "bound to a Secret",
		},
		{
			name:    "prohibited value",
			update:  map[string]map[string]*string{"api": {"BLOCKED": new("override")}},
			wantErr: "not user-allowed",
		},
		{
			name:    "value without a policy",
			update:  map[string]map[string]*string{"api": {"UNKNOWN": new("override")}},
			wantErr: "not user-allowed",
		},
		{
			name:    "unknown component",
			update:  map[string]map[string]*string{"missing": {"REGION": new("us-east")}},
			wantErr: `unknown component ID "missing"`,
		},
		{
			name:    "URL on another hostname",
			update:  map[string]map[string]*string{"remote": {"__url": new("https://attacker.example.net/mcp")}},
			wantErr: "invalid URL",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			set, cleared, err := ValidateProfileConfiguration(components, tt.update)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				httpErr, ok := err.(*types.ErrHTTP)
				require.True(t, ok, "error %T is not an API error", err)
				require.Equal(t, http.StatusBadRequest, httpErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSet, set)
			require.ElementsMatch(t, tt.wantCleared, cleared)
		})
	}
}

// newProfileConfigurationTestVMCP returns a stored shared vMCP whose profiles name agent 7, every agent, and every
// user, as the vMCP controller has last seen it.
func newProfileConfigurationTestVMCP(t *testing.T, objects ...kclient.Object) (kclient.WithWatch, *v1.VMCP) {
	t.Helper()
	vmcp := &v1.VMCP{
		Name:      "vmcp1profiles",
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Profiles",
			Components:  []types.VMCPComponent{profileConfiguredComponent("api"), hostnameComponent("remote")},
			Profiles: []types.VMCPProfile{
				explicitAgentProfile(profileExplicit, "7"),
				everyAgentProfile(profileAgents, types.SubjectTypeAgent),
				everyAgentProfile(profileUsers, types.SubjectTypeUser),
			},
		}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(append([]kclient.Object{vmcp}, objects...)...).Build()
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
	vmcp.Status.ProfileIDs = ProfileIDs(vmcp.Spec.Manifest)
	require.NoError(t, client.Status().Update(t.Context(), vmcp))
	return client, vmcp
}

func getVMCP(t *testing.T, client kclient.Client, vmcp *v1.VMCP) *v1.VMCP {
	t.Helper()
	current := &v1.VMCP{}
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
	return current
}

func requireHTTPError(t *testing.T, err error, code int, contains string) {
	t.Helper()
	httpErr, ok := err.(*types.ErrHTTP)
	require.True(t, ok, "error %v (%T) is not an API error", err, err)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
	require.Contains(t, httpErr.Message, contains)
}

func TestProfileConfigurationStoreUpdate(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	credentials := newMemoryCredentials()
	store := NewProfileConfigurationStore(credentials)
	key := kclient.ObjectKeyFromObject(vmcp)

	updated, err := store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{
		"api":    {"REGION": new("secret-region"), "VERBOSE": new("secret-verbose")},
		"remote": {"__url": new("https://tenant.example.com/secret")},
	})
	require.NoError(t, err)
	first := updated.Status.ProfileConfigurationRevisions[profileExplicit]
	require.True(t, strings.HasPrefix(first.Name, "configuration-"), first.Name)
	require.Equal(t, []string{ConfigurationKey("api", "REGION"), ConfigurationKey("api", "VERBOSE"), ConfigurationKey("remote", "__url")}, first.Keys)
	require.Empty(t, first.Superseded)
	require.Equal(t, []string{first.Name}, credentials.revisions(vmcp.Name, profileExplicit))
	values, err := store.RevisionValues(t.Context(), vmcp.Name, profileExplicit, first.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		ConfigurationKey("api", "REGION"):   "secret-region",
		ConfigurationKey("api", "VERBOSE"):  "secret-verbose",
		ConfigurationKey("remote", "__url"): "https://tenant.example.com/secret",
	}, values)

	// The status that publishes the revision carries key names, never values.
	stored, err := json.Marshal(getVMCP(t, client, vmcp))
	require.NoError(t, err)
	for _, secret := range []string{"secret-region", "secret-verbose", "tenant.example.com/secret"} {
		require.NotContains(t, string(stored), secret)
	}
	status := ProfileStatus(*updated, updated.Spec.Manifest.Profiles[0])
	require.True(t, status.AgentCapable)
	require.Equal(t, []types.VMCPProfileComponentStatus{
		{ComponentID: "api", Configuration: types.VMCPProfileConfigurationStatus{Set: []string{"REGION", "VERBOSE"}, MissingRequired: []string{}, OptionalUnset: []string{}}},
		{ComponentID: "remote", Configuration: types.VMCPProfileConfigurationStatus{Set: []string{"__url"}, MissingRequired: []string{}, OptionalUnset: []string{}}},
	}, status.Components)

	// null clears a key, and keys left out keep their values. The replaced revision is superseded, not deleted.
	updated, err = store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"VERBOSE": nil}})
	require.NoError(t, err)
	second := updated.Status.ProfileConfigurationRevisions[profileExplicit]
	require.NotEqual(t, first.Name, second.Name)
	require.Equal(t, []string{ConfigurationKey("api", "REGION"), ConfigurationKey("remote", "__url")}, second.Keys)
	require.Equal(t, []string{first.Name}, second.Superseded)
	values, err = store.RevisionValues(t.Context(), vmcp.Name, profileExplicit, second.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		ConfigurationKey("api", "REGION"):   "secret-region",
		ConfigurationKey("remote", "__url"): "https://tenant.example.com/secret",
	}, values)

	// An update that changes nothing publishes nothing.
	version := getVMCP(t, client, vmcp).ResourceVersion
	_, err = store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"REGION": new("secret-region"), "VERBOSE": nil}})
	require.NoError(t, err)
	require.Equal(t, version, getVMCP(t, client, vmcp).ResourceVersion)
	require.Len(t, credentials.revisions(vmcp.Name, profileExplicit), 2)

	// Writes are refused for a profile that does not apply to agents, a profile the vMCP lacks, and a key a profile
	// cannot set, and none of them stores anything.
	_, err = store.Update(t.Context(), client, key, profileUsers, map[string]map[string]*string{"api": {"REGION": new("x")}})
	requireHTTPError(t, err, http.StatusBadRequest, "does not apply to agents")
	_, err = store.Update(t.Context(), client, key, "vp1missing", map[string]map[string]*string{"api": {"REGION": new("x")}})
	requireHTTPError(t, err, http.StatusNotFound, "no profile")
	_, err = store.Update(t.Context(), client, key, profileAgents, map[string]map[string]*string{"api": {"ADMIN_TOKEN": new("x")}})
	requireHTTPError(t, err, http.StatusBadRequest, "fixed by the vMCP")
	_, err = store.Update(t.Context(), client, kclient.ObjectKey{Namespace: "default", Name: "vmcp1missing"}, profileExplicit, nil)
	requireHTTPError(t, err, http.StatusNotFound, "not found")
	require.Empty(t, credentials.revisions(vmcp.Name, profileUsers))
	require.Empty(t, credentials.revisions(vmcp.Name, profileAgents))

	// Reset clears the configuration, keeping the revisions until no instance uses them.
	updated, cleared, err := store.Reset(t.Context(), client, key, profileExplicit)
	require.NoError(t, err)
	require.True(t, cleared)
	require.Equal(t, v1.VMCPProfileConfigurationRevision{Superseded: []string{first.Name, second.Name}}, updated.Status.ProfileConfigurationRevisions[profileExplicit])
	status = ProfileStatus(*updated, updated.Spec.Manifest.Profiles[0])
	require.True(t, status.PendingCollection)
	require.Equal(t, []string{"REGION"}, status.Components[0].Configuration.MissingRequired)
	require.Equal(t, []string{"VERBOSE"}, status.Components[0].Configuration.OptionalUnset)
	require.Equal(t, []string{"__url"}, status.Components[1].Configuration.MissingRequired)
	// Resetting again changes nothing.
	version = getVMCP(t, client, vmcp).ResourceVersion
	_, cleared, err = store.Reset(t.Context(), client, key, profileExplicit)
	require.NoError(t, err)
	require.False(t, cleared)
	require.Equal(t, version, getVMCP(t, client, vmcp).ResourceVersion)
}

// TestProfileConfigurationResetIsAllowedWhereUpdatesAreNot checks that configuration can be removed from a profile that
// no longer applies to agents, and from a vMCP whose agent selection became ambiguous, where it can no longer be set.
func TestProfileConfigurationResetIsAllowedWhereUpdatesAreNot(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	store := NewProfileConfigurationStore(newMemoryCredentials())
	key := kclient.ObjectKeyFromObject(vmcp)
	_, err := store.Update(t.Context(), client, key, profileAgents, map[string]map[string]*string{"api": {"REGION": new("x")}})
	require.NoError(t, err)

	// The profile that named every agent now names every user, and a second profile names everyone: agent selection
	// is ambiguous too.
	current := getVMCP(t, client, vmcp)
	current.Spec.Manifest.Profiles[1].Subjects = []types.Subject{{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}}
	current.Spec.Manifest.Profiles = append(current.Spec.Manifest.Profiles,
		everyAgentProfile("vp1everyone", types.SubjectTypeSelector), everyAgentProfile("vp1everyonetoo", types.SubjectTypeSelector))
	require.NoError(t, client.Update(t.Context(), current))

	_, err = store.Update(t.Context(), client, key, profileAgents, map[string]map[string]*string{"api": {"REGION": new("y")}})
	requireHTTPError(t, err, http.StatusBadRequest, "does not apply to agents")
	_, err = store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"REGION": new("y")}})
	requireHTTPError(t, err, http.StatusConflict, "more than one profile")

	updated, cleared, err := store.Reset(t.Context(), client, key, profileAgents)
	require.NoError(t, err)
	require.True(t, cleared)
	require.Empty(t, updated.Status.ProfileConfigurationRevisions[profileAgents].Name)
}

// TestProfileConfigurationWriteRecordsTheProfileFirst checks that a profile's ID is in the vMCP's status, recorded at a
// resource version that has the profile, before anything is stored for it, so that the vMCP controller deletes it with
// the profile however soon the profile is removed.
func TestProfileConfigurationWriteRecordsTheProfileFirst(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	// The profile was just added, and the controller has not recorded it yet.
	current := getVMCP(t, client, vmcp)
	current.Status.ProfileIDs = slices.DeleteFunc(current.Status.ProfileIDs, func(id string) bool { return id == profileAgents })
	require.NoError(t, client.Status().Update(t.Context(), current))

	credentials := newMemoryCredentials()
	var checked bool
	credentials.onUpsert = func(gatewaytypes.Credential) {
		stored := getVMCP(t, client, vmcp)
		require.Contains(t, stored.Status.ProfileIDs, profileAgents, "configuration was stored before its profile was recorded")
		checked = true
	}
	_, err := NewProfileConfigurationStore(credentials).Update(t.Context(), client, kclient.ObjectKeyFromObject(vmcp), profileAgents, map[string]map[string]*string{"api": {"REGION": new("x")}})
	require.NoError(t, err)
	require.True(t, checked)
}

// TestProfileConfigurationWriteDeletesUnpublishedRevision removes the profile while its configuration is being written:
// the write is refused, and leaves no revision behind, even though the controller cleaned up the profile before the
// revision was stored.
func TestProfileConfigurationWriteDeletesUnpublishedRevision(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	credentials := newMemoryCredentials()
	store := NewProfileConfigurationStore(credentials)
	credentials.onUpsert = func(gatewaytypes.Credential) {
		credentials.onUpsert = nil
		current := getVMCP(t, client, vmcp)
		current.Spec.Manifest.Profiles = current.Spec.Manifest.Profiles[1:]
		require.NoError(t, client.Update(t.Context(), current))
		// The controller deletes the removed profile's data and records the removal, before this write stores its
		// revision.
		require.NoError(t, store.DeleteProfileData(t.Context(), vmcp.Name, []string{profileExplicit}))
		current = getVMCP(t, client, vmcp)
		current.Status.ProfileIDs = ProfileIDs(current.Spec.Manifest)
		require.NoError(t, client.Status().Update(t.Context(), current))
	}

	_, err := store.Update(t.Context(), client, kclient.ObjectKeyFromObject(vmcp), profileExplicit, map[string]map[string]*string{"api": {"REGION": new("x")}})
	requireHTTPError(t, err, http.StatusNotFound, "no profile")
	require.Empty(t, credentials.revisions(vmcp.Name, profileExplicit))
	require.NotContains(t, getVMCP(t, client, vmcp).Status.ProfileConfigurationRevisions, profileExplicit)
}

// TestProfileConfigurationConcurrentWritesMerge checks that a write that loses a race with another is applied on top of
// it, rather than over a stale read, and that its unpublished revision is deleted.
func TestProfileConfigurationConcurrentWritesMerge(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	credentials := newMemoryCredentials()
	store := NewProfileConfigurationStore(credentials)
	key := kclient.ObjectKeyFromObject(vmcp)
	credentials.onUpsert = func(gatewaytypes.Credential) {
		credentials.onUpsert = nil
		_, err := store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"VERBOSE": new("winner")}})
		require.NoError(t, err)
	}

	updated, err := store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"REGION": new("loser")}})
	require.NoError(t, err)
	current := updated.Status.ProfileConfigurationRevisions[profileExplicit]
	values, err := store.RevisionValues(t.Context(), vmcp.Name, profileExplicit, current.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{ConfigurationKey("api", "REGION"): "loser", ConfigurationKey("api", "VERBOSE"): "winner"}, values)
	// The winner's revision is superseded, and the loser's first attempt, never published, is gone.
	require.Len(t, current.Superseded, 1)
	require.ElementsMatch(t, []string{current.Name, current.Superseded[0]}, credentials.revisions(vmcp.Name, profileExplicit))
}

// TestProfileConfigurationStoreIsAProfileDataStore checks the profile data hook with real data: which profiles have
// configuration, in one query, and deleting it, which can be repeated.
func TestProfileConfigurationStoreIsAProfileDataStore(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	credentials := newMemoryCredentials()
	store := NewProfileConfigurationStore(credentials)
	key := kclient.ObjectKeyFromObject(vmcp)
	for _, profileID := range []string{profileExplicit, profileAgents} {
		_, err := store.Update(t.Context(), client, key, profileID, map[string]map[string]*string{"api": {"REGION": new("x")}})
		require.NoError(t, err)
	}
	// Another vMCP's profile of the same ID is another profile.
	require.NoError(t, credentials.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: ProfileConfigurationCredentialContext("vmcp1other", profileUsers), Name: "configuration-x"}))

	queries := credentials.contextQueries
	withData, err := store.ProfilesWithData(t.Context(), vmcp.Name, []string{profileUsers, profileAgents, profileExplicit, "vp1missing"})
	require.NoError(t, err)
	require.Equal(t, []string{profileAgents, profileExplicit}, withData)
	require.Equal(t, queries+1, credentials.contextQueries)

	require.NoError(t, store.DeleteProfileData(t.Context(), vmcp.Name, []string{profileExplicit, "vp1missing"}))
	require.NoError(t, store.DeleteProfileData(t.Context(), vmcp.Name, []string{profileExplicit, "vp1missing"}))
	withData, err = store.ProfilesWithData(t.Context(), vmcp.Name, []string{profileUsers, profileAgents, profileExplicit})
	require.NoError(t, err)
	require.Equal(t, []string{profileAgents}, withData)
	withData, err = store.ProfilesWithData(t.Context(), "vmcp1other", []string{profileUsers})
	require.NoError(t, err)
	require.Equal(t, []string{profileUsers}, withData)
}

func TestCollectSupersededProfileConfiguration(t *testing.T) {
	credentials := newMemoryCredentials()
	store := NewProfileConfigurationStore(credentials)
	vmcp := &v1.VMCP{Name: "vmcp1collect"}
	vmcp.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{
		profileExplicit: {Name: "configuration-current", Keys: []string{"k"}, Superseded: []string{"configuration-used", "configuration-unused"}},
		profileAgents:   {Superseded: []string{"configuration-reset"}},
	}
	for profileID, names := range map[string][]string{
		profileExplicit: {"configuration-current", "configuration-used", "configuration-unused"},
		profileAgents:   {"configuration-reset"},
	} {
		for _, name := range names {
			require.NoError(t, credentials.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: ProfileConfigurationCredentialContext(vmcp.Name, profileID), Name: name}))
		}
	}
	instances := []v1.VMCPInstance{
		{
			Spec:   v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
			Status: v1.VMCPInstanceStatus{AgentConfigurationRevisions: map[string]string{profileExplicit: "configuration-used"}},
		},
		{
			// An instance of another vMCP keeps nothing of this one's.
			Spec:   v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1other"}},
			Status: v1.VMCPInstanceStatus{AgentConfigurationRevisions: map[string]string{profileAgents: "configuration-reset"}},
		},
	}

	changed, err := store.CollectSuperseded(t.Context(), vmcp, instances)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, map[string]v1.VMCPProfileConfigurationRevision{
		profileExplicit: {Name: "configuration-current", Keys: []string{"k"}, Superseded: []string{"configuration-used"}},
	}, vmcp.Status.ProfileConfigurationRevisions)
	require.Equal(t, []string{"configuration-current", "configuration-used"}, credentials.revisions(vmcp.Name, profileExplicit))
	require.Empty(t, credentials.revisions(vmcp.Name, profileAgents))

	// Once the instance moves to the current revision, the last superseded one goes too.
	instances[0].Status.AgentConfigurationRevisions[profileExplicit] = "configuration-current"
	changed, err = store.CollectSuperseded(t.Context(), vmcp, instances)
	require.NoError(t, err)
	require.True(t, changed)
	require.Empty(t, vmcp.Status.ProfileConfigurationRevisions[profileExplicit].Superseded)
	require.Equal(t, []string{"configuration-current"}, credentials.revisions(vmcp.Name, profileExplicit))
	changed, err = store.CollectSuperseded(t.Context(), vmcp, instances)
	require.NoError(t, err)
	require.False(t, changed)
}

// TestAgentConfigurationValuesStayWithTheirProfiles checks that an agent is given the values of the profiles that name
// it and nothing else: one profile's values never reach an agent that resolves only to another, and the wildcard's
// reach an agent only for what its own profile lacks.
func TestAgentConfigurationValuesStayWithTheirProfiles(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	current := getVMCP(t, client, vmcp)
	current.Spec.Manifest.Profiles = append(current.Spec.Manifest.Profiles, explicitAgentProfile(profileOther, "8"))
	require.NoError(t, client.Update(t.Context(), current))
	store := NewProfileConfigurationStore(newMemoryCredentials())
	key := kclient.ObjectKeyFromObject(vmcp)
	for profileID, update := range map[string]map[string]map[string]*string{
		profileExplicit: {"api": {"REGION": new("seven-region"), "VERBOSE": new("seven-verbose")}},
		profileOther:    {"api": {"VERBOSE": new("eight-verbose")}},
		profileAgents:   {"api": {"REGION": new("agents-region"), "VERBOSE": new("agents-verbose")}, "remote": {"__url": new("https://tenant.example.com/agents")}},
	} {
		_, err := store.Update(t.Context(), client, key, profileID, update)
		require.NoError(t, err)
	}
	stored := getVMCP(t, client, vmcp)

	for _, tt := range []struct {
		name          string
		agent         uint
		want          map[string]string
		wantFallback  []string
		wantRevisions []string
	}{
		{
			name:  "agent with a profile that supplies everything but the URL",
			agent: 7,
			want: map[string]string{
				ConfigurationKey("api", "REGION"):   "seven-region",
				ConfigurationKey("api", "VERBOSE"):  "seven-verbose",
				ConfigurationKey("remote", "__url"): "https://tenant.example.com/agents",
			},
			wantFallback:  []string{profileAgents},
			wantRevisions: []string{profileAgents, profileExplicit},
		},
		{
			name:  "agent whose profile lacks a required value",
			agent: 8,
			want: map[string]string{
				ConfigurationKey("api", "REGION"):   "agents-region",
				ConfigurationKey("api", "VERBOSE"):  "eight-verbose",
				ConfigurationKey("remote", "__url"): "https://tenant.example.com/agents",
			},
			wantFallback:  []string{profileAgents},
			wantRevisions: []string{profileAgents, profileOther},
		},
		{
			name:  "agent without a profile of its own",
			agent: 9,
			want: map[string]string{
				ConfigurationKey("api", "REGION"):   "agents-region",
				ConfigurationKey("api", "VERBOSE"):  "agents-verbose",
				ConfigurationKey("remote", "__url"): "https://tenant.example.com/agents",
			},
			wantFallback:  []string{profileAgents},
			wantRevisions: []string{profileAgents},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configuration := ResolveAgentConfiguration(*stored, v1.VMCPInstance{}, agentInfo(tt.agent, nil))
			values, err := store.AgentConfigurationValues(t.Context(), stored.Name, configuration)
			require.NoError(t, err)
			require.Equal(t, tt.want, values)
			require.Equal(t, tt.wantFallback, configuration.FallbackProfileIDs())
			require.Equal(t, tt.wantRevisions, slices.Sorted(maps.Keys(configuration.Revisions)))
		})
	}

	// The revisions differ per agent, and follow a change only for the agents it reaches.
	seven := ResolveAgentConfiguration(*stored, v1.VMCPInstance{}, agentInfo(7, nil)).Revision
	eight := ResolveAgentConfiguration(*stored, v1.VMCPInstance{}, agentInfo(8, nil)).Revision
	require.NotEqual(t, seven, eight)
	_, err := store.Update(t.Context(), client, key, profileOther, map[string]map[string]*string{"api": {"VERBOSE": new("eight-verbose-2")}})
	require.NoError(t, err)
	changed := getVMCP(t, client, vmcp)
	require.Equal(t, seven, ResolveAgentConfiguration(*changed, v1.VMCPInstance{}, agentInfo(7, nil)).Revision)
	require.NotEqual(t, eight, ResolveAgentConfiguration(*changed, v1.VMCPInstance{}, agentInfo(8, nil)).Revision)
}

// TestRevisionValuesMissing checks that a published revision that has gone is an error to retry, never an empty
// configuration.
func TestRevisionValuesMissing(t *testing.T) {
	values, err := NewProfileConfigurationStore(newMemoryCredentials()).RevisionValues(t.Context(), "vmcp1test", profileExplicit, "configuration-gone")
	var notFound gateway.CredentialNotFoundError
	require.ErrorAs(t, err, &notFound)
	require.Nil(t, values)
}
