package vmcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	profileConfigurationContextPrefix = "vmcp-profile/"
)

var (
	_ ProfileDataStore = (*ProfileConfigurationStore)(nil)
)

// ProfileConfigurationCredentials are the credential operations that profile configuration is stored with. The gateway
// client implements them.
type ProfileConfigurationCredentials interface {
	RevealCredential(ctx context.Context, contexts []string, name string) (gatewaytypes.Credential, error)
	UpsertCredential(ctx context.Context, credential gatewaytypes.Credential) error
	DeleteCredential(ctx context.Context, credentialContext, name string) (bool, error)
	CredentialContextsWithCredentials(ctx context.Context, contexts []string) ([]string, error)
	DeleteCredentialsInContexts(ctx context.Context, contexts []string) error
}

// ProfileConfigurationStore stores the configuration that administrators set on agent-capable vMCP profiles, which
// the agents the profiles apply to are given in place of the values a user enters on their own connection.
//
// A profile's configuration is kept in revisions: credentials in the profile's own context
// (ProfileConfigurationCredentialContext), keyed like a vMCP's configuration (ConfigurationKey). Every change writes a
// new revision and then publishes it, with the keys it sets, in the vMCP's status
// (v1.VMCPStatus.ProfileConfigurationRevisions); the revision it replaces is recorded there as superseded, and is
// deleted once no agent's instance records that its configuration was copied from it (CollectSuperseded). Nothing about
// it is in the manifest, so catalog-synced vMCPs keep it across syncs. It is a ProfileDataStore: the vMCP controller
// deletes a profile's revisions when the profile or its vMCP is deleted.
type ProfileConfigurationStore struct {
	credentials ProfileConfigurationCredentials
}

// profileConfigurationChange computes a profile's next configuration from its current revision, whose values it reads
// only if it needs them, and reports whether that is a change.
type profileConfigurationChange func(vmcp *v1.VMCP, profile types.VMCPProfile, current v1.VMCPProfileConfigurationRevision, values func() (map[string]string, error)) (next map[string]string, changed bool, err error)

// profileDataWrite is one write of a profile's data by writeProfileData.
type profileDataWrite struct {
	// record records in the vMCP's status, besides the profile's ID, what the vMCP controller needs to delete the data
	// once it is stored, and reports whether that changed the status. It may be nil.
	record func(vmcp *v1.VMCP) bool
	// store stores the data.
	store func(ctx context.Context) error
	// publish publishes the stored data in the vMCP's status.
	publish func(vmcp *v1.VMCP)
	// discard deletes the stored data once a conflict proves it was not published, logging its own failure.
	discard func(ctx context.Context)
}

func NewProfileConfigurationStore(credentials ProfileConfigurationCredentials) *ProfileConfigurationStore {
	return &ProfileConfigurationStore{credentials: credentials}
}

// ProfileConfigurationCredentialContext is the credential context of one vMCP profile's configuration revisions. No
// other profile, no vMCP and no instance stores anything in it, so the values of one profile are never read as
// another's.
func ProfileConfigurationCredentialContext(vmcpID, profileID string) string {
	return profileConfigurationContextPrefix + vmcpID + "/" + profileID
}

// newProfileConfigurationRevision names a new revision of a profile's configuration. Static configuration revisions
// are named by a digest of their values; these are named at random instead, so that a name published in status reveals
// nothing about the values, and so that two writes never share a revision.
func newProfileConfigurationRevision() string {
	id := make([]byte, 16)
	// crypto/rand.Read never fails; it crashes the program instead.
	_, _ = rand.Read(id)
	return configurationCredentialName + "-" + hex.EncodeToString(id)
}

// ProfilesWithData returns the profiles, among profileIDs, that have any configuration revision stored, with one query.
func (s *ProfileConfigurationStore) ProfilesWithData(ctx context.Context, vmcpID string, profileIDs []string) ([]string, error) {
	if len(profileIDs) == 0 {
		return nil, nil
	}
	contexts := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		contexts = append(contexts, ProfileConfigurationCredentialContext(vmcpID, id))
	}
	found, err := s.credentials.CredentialContextsWithCredentials(ctx, contexts)
	if err != nil {
		return nil, fmt.Errorf("find vMCP %q profiles with configuration: %w", vmcpID, err)
	}
	var result []string
	for _, id := range profileIDs {
		if slices.Contains(found, ProfileConfigurationCredentialContext(vmcpID, id)) && !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result, nil
}

// DeleteProfileData deletes every configuration revision of the profiles, with one statement. Deleting what is already
// gone succeeds, so it can be called again after any failure.
func (s *ProfileConfigurationStore) DeleteProfileData(ctx context.Context, vmcpID string, profileIDs []string) error {
	contexts := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		contexts = append(contexts, ProfileConfigurationCredentialContext(vmcpID, id))
	}
	if err := s.credentials.DeleteCredentialsInContexts(ctx, contexts); err != nil {
		return fmt.Errorf("delete configuration of vMCP %q profiles: %w", vmcpID, err)
	}
	return nil
}

// RevisionValues returns the values that a profile's configuration revision stores, keyed by ConfigurationKey. A
// published revision always exists, unless it was superseded and deleted after the caller read the vMCP, so a missing
// one is an error to retry with the vMCP's current status, never an empty configuration.
func (s *ProfileConfigurationStore) RevisionValues(ctx context.Context, vmcpID, profileID, revision string) (map[string]string, error) {
	credential, err := s.credentials.RevealCredential(ctx, []string{ProfileConfigurationCredentialContext(vmcpID, profileID)}, revision)
	if err != nil {
		return nil, fmt.Errorf("read configuration revision %q of vMCP %q profile %q: %w", revision, vmcpID, profileID, err)
	}
	return credential.Secrets, nil
}

// AgentConfigurationValues returns the values that configuration resolves an agent to, keyed by ConfigurationKey. Each
// is read from the revision of the profile its source names, which is one of the profiles that name the agent, so no
// other profile's values are ever read for it.
func (s *ProfileConfigurationStore) AgentConfigurationValues(ctx context.Context, vmcpID string, configuration AgentConfiguration) (map[string]string, error) {
	revealed := make(map[string]map[string]string, len(configuration.Revisions))
	for profileID, revision := range configuration.Revisions {
		values, err := s.RevisionValues(ctx, vmcpID, profileID, revision)
		if err != nil {
			return nil, err
		}
		revealed[profileID] = values
	}
	result := map[string]string{}
	for _, component := range configuration.Components {
		for _, source := range component.Sources {
			key := ConfigurationKey(component.ComponentID, source.Field)
			if value := revealed[source.ProfileID][key]; value != "" {
				result[key] = value
			} else {
				// The status that published the revision lists the key, so the revision should store it. Leaving it out
				// fails closed: the component's server reports the value missing.
				slog.Warn("Profile configuration revision lacks a key its status lists", "vmcp", vmcpID, "profile", source.ProfileID, "revision", source.Revision, "component", component.ComponentID, "key", source.Field)
			}
		}
	}
	return result, nil
}

// Update changes the configuration of a vMCP's profile by component ID and key: a value sets the key, nil (or a blank
// value) clears it, and keys left out keep their values. The profile must be agent-capable. Every component changed
// must be one the profile grants, since a profile's configuration is only used for the components it grants
// (profileForComponent). Only the values a user would enter on their own connection can be set
// (ValidateProfileConfiguration). Values the profile kept for components it no longer grants are carried on as they
// are. It returns the vMCP with the new configuration published.
func (s *ProfileConfigurationStore) Update(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID string, update map[string]map[string]*string) (*v1.VMCP, error) {
	return s.write(ctx, client, key, profileID, func(vmcp *v1.VMCP, profile types.VMCPProfile, current v1.VMCPProfileConfigurationRevision, values func() (map[string]string, error)) (map[string]string, bool, error) {
		if err := checkProfileSetup(profile, "configuration"); err != nil {
			return nil, false, err
		}
		components := ComponentsForInstance(*vmcp, v1.VMCPInstance{})
		for _, componentID := range slices.Sorted(maps.Keys(update)) {
			if slices.ContainsFunc(components, func(component types.VMCPComponent) bool { return component.ID == componentID }) {
				if err := checkProfileGrants(profile, componentID, "configuration"); err != nil {
					return nil, false, err
				}
			}
		}
		set, cleared, err := ValidateProfileConfiguration(components, update)
		if err != nil {
			return nil, false, err
		}
		existing, err := values()
		if err != nil {
			return nil, false, err
		}
		// Values the profile can no longer set, such as one the vMCP now fixes, are dropped rather than carried on.
		next := configurableValues(components, existing)
		for _, key := range cleared {
			delete(next, key)
		}
		maps.Copy(next, set)
		return next, current.Name == "" && len(next) > 0 || current.Name != "" && !maps.Equal(next, existing), nil
	})
}

// Reset clears the configuration of a vMCP's profile, and reports whether the profile had any to clear. Unlike Update,
// it is allowed on a profile that is not agent-capable: removing configuration can never give an agent the wrong
// values.
func (s *ProfileConfigurationStore) Reset(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID string) (*v1.VMCP, bool, error) {
	var cleared bool
	vmcp, err := s.write(ctx, client, key, profileID, func(_ *v1.VMCP, _ types.VMCPProfile, current v1.VMCPProfileConfigurationRevision, _ func() (map[string]string, error)) (map[string]string, bool, error) {
		// The current revision is not read, so a reset works even if it is unreadable.
		cleared = current.Name != ""
		return nil, cleared, nil
	})
	return vmcp, cleared, err
}

// write publishes a profile's next configuration, in the order that keeps every revision deleted with its profile
// (writeProfileData). The new revision is published with the keys it sets, and nothing reads it until then. The
// revision it replaces is recorded as superseded, and is deleted by the vMCP controller once no instance uses it.
func (s *ProfileConfigurationStore) write(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID string, change profileConfigurationChange) (*v1.VMCP, error) {
	return writeProfileData(ctx, client, key, profileID, func(vmcp *v1.VMCP) (*profileDataWrite, error) {
		profile, err := writableProfile(vmcp, profileID)
		if err != nil {
			return nil, err
		}
		current := vmcp.Status.ProfileConfigurationRevisions[profileID]
		values := func() (map[string]string, error) {
			if current.Name == "" {
				return map[string]string{}, nil
			}
			return s.RevisionValues(ctx, vmcp.Name, profileID, current.Name)
		}
		next, changed, err := change(vmcp, profile, current, values)
		if err != nil || !changed {
			return nil, err
		}

		credentialContext := ProfileConfigurationCredentialContext(vmcp.Name, profileID)
		var revision string
		if len(next) > 0 {
			revision = newProfileConfigurationRevision()
		}
		return &profileDataWrite{
			store: func(ctx context.Context) error {
				if revision == "" {
					return nil
				}
				if err := s.credentials.UpsertCredential(ctx, gatewaytypes.Credential{
					Context: credentialContext,
					Name:    revision,
					Secrets: next,
				}); err != nil {
					return fmt.Errorf("store configuration of vMCP %q profile %q: %w", vmcp.Name, profileID, err)
				}
				return nil
			},
			publish: func(vmcp *v1.VMCP) {
				published := v1.VMCPProfileConfigurationRevision{
					Name:       revision,
					Keys:       slices.Sorted(maps.Keys(next)),
					Superseded: slices.Clone(current.Superseded),
				}
				if current.Name != "" {
					published.Superseded = append(published.Superseded, current.Name)
				}
				setProfileConfigurationRevision(vmcp, profileID, published)
			},
			discard: func(ctx context.Context) {
				if revision == "" {
					return
				}
				if _, err := s.credentials.DeleteCredential(ctx, credentialContext, revision); err != nil {
					slog.Warn("Failed to delete an unpublished profile configuration revision", "vmcp", vmcp.Name, "profile", profileID, "revision", revision, "error", err)
				}
			},
		}, nil
	})
}

// writeProfileData writes data of the profile with profileID of the vMCP with key, the profile's configuration or an
// OAuth grant, and returns the vMCP with it published. prepare is called with the vMCP as stored on every attempt, and
// returns the write, nil when there is nothing to write, or the error that refuses it. Conflicting writes, by other
// administrators or by controllers, are retried from a fresh read, so no write is lost and none is applied over a
// stale one.
//
// The order of its steps keeps the data deleted with its profile:
//
//  1. Before anything is stored, the profile's ID is recorded in the vMCP's status (v1.VMCPStatus.ProfileIDs), with
//     what the write records, in a status update at a resource version whose spec has the profile. The vMCP controller
//     deletes the data of every recorded profile that leaves the spec, so a profile removed from now on takes its data
//     with it, even one added and removed before the controller saw it.
//  2. The data is stored.
//  3. It is published in a status update at the same resource version. A conflict means the vMCP changed, possibly by
//     losing the profile, and possibly after the controller deleted the profile's data, so the unpublished data is
//     discarded before the write is retried. Nothing reads the data until it is published.
func writeProfileData(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID string, prepare func(vmcp *v1.VMCP) (*profileDataWrite, error)) (*v1.VMCP, error) {
	var result *v1.VMCP
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		vmcp, err := readVMCP(ctx, client, key)
		if err != nil {
			return err
		}
		write, err := prepare(vmcp)
		if err != nil {
			return err
		}
		if write == nil {
			result = vmcp
			return nil
		}

		recorded := false
		if !slices.Contains(vmcp.Status.ProfileIDs, profileID) {
			vmcp.Status.ProfileIDs = append(vmcp.Status.ProfileIDs, profileID)
			recorded = true
		}
		if write.record != nil && write.record(vmcp) {
			recorded = true
		}
		if recorded {
			if err := client.Status().Update(ctx, vmcp); err != nil {
				return err
			}
		}

		if err := write.store(ctx); err != nil {
			return err
		}

		write.publish(vmcp)
		if err := client.Status().Update(ctx, vmcp); err != nil {
			// Only a conflict proves the update was not applied; after any other error the data may be published.
			if apierrors.IsConflict(err) {
				write.discard(ctx)
			}
			return err
		}
		result = vmcp
		return nil
	})
	return result, err
}

// readVMCP reads the vMCP with key, mapping its absence to a 404. A write of profile data must read it from storage,
// not a cache.
func readVMCP(ctx context.Context, client kclient.Client, key kclient.ObjectKey) (*v1.VMCP, error) {
	var vmcp v1.VMCP
	if err := client.Get(ctx, key, &vmcp); apierrors.IsNotFound(err) {
		return nil, types.NewErrNotFound("vMCP %q not found", key.Name)
	} else if err != nil {
		return nil, err
	}
	return &vmcp, nil
}

// profileOf returns the profile of vmcp with profileID, or a 404 when it has none. A personal vMCP has no profiles;
// this guards a stored one that does all the same.
func profileOf(vmcp *v1.VMCP, profileID string) (types.VMCPProfile, error) {
	profile, ok := vmcp.Spec.Manifest.Profile(profileID)
	if !ok || vmcp.Spec.UserID != "" {
		return types.VMCPProfile{}, types.NewErrNotFound("vMCP %q has no profile %q", vmcp.Name, profileID)
	}
	return profile, nil
}

// writableProfile returns the profile of vmcp with profileID that an administrator writes data of, or the error that
// refuses it: the vMCP is being deleted, or it has no such profile.
func writableProfile(vmcp *v1.VMCP, profileID string) (types.VMCPProfile, error) {
	if !vmcp.DeletionTimestamp.IsZero() {
		return types.VMCPProfile{}, types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("vMCP %q is being deleted", vmcp.Name))
	}
	return profileOf(vmcp, profileID)
}

// checkProfileSetup refuses to set up what, a kind of profile data such as configuration or OAuth grants, on profile
// for agents, unless the profile is agent-capable: only the agents a profile names use its setup.
func checkProfileSetup(profile types.VMCPProfile, what string) error {
	if !profile.IsAgentCapable() {
		return types.NewErrBadRequest("profile %q names no agent; only a profile that names an agent can hold %s for it", profile.ID, what)
	}
	return nil
}

// checkProfileGrants refuses to set up what, a kind of profile data, on profile for the component with componentID
// unless the profile grants it, since a profile's data is only used for the components it grants
// (profileForComponent).
func checkProfileGrants(profile types.VMCPProfile, componentID, what string) error {
	if !ProfileGrantsComponent(profile, componentID) {
		return types.NewErrBadRequest("profile %q does not grant component %q; a profile holds %s only for the components it grants", profile.ID, componentID, what)
	}
	return nil
}

// setProfileConfigurationRevision records a profile's configuration in a vMCP's status, leaving out a profile that has
// neither configuration nor superseded revisions.
func setProfileConfigurationRevision(vmcp *v1.VMCP, profileID string, revision v1.VMCPProfileConfigurationRevision) {
	if revision.Name == "" && len(revision.Superseded) == 0 {
		delete(vmcp.Status.ProfileConfigurationRevisions, profileID)
		return
	}
	if vmcp.Status.ProfileConfigurationRevisions == nil {
		vmcp.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{}
	}
	vmcp.Status.ProfileConfigurationRevisions[profileID] = revision
}

// CollectSuperseded deletes the superseded configuration revisions of vmcp's profiles that no instance among instances
// records that its configuration was copied from, and removes them from vmcp's status. It reports whether it changed
// the status, which the caller saves. A revision deleted while an instance is still being resolved from it fails that
// instance's sync, which is retried from the vMCP's current revision.
func (s *ProfileConfigurationStore) CollectSuperseded(ctx context.Context, vmcp *v1.VMCP, instances []v1.VMCPInstance) (bool, error) {
	var changed bool
	for _, profileID := range slices.Sorted(maps.Keys(vmcp.Status.ProfileConfigurationRevisions)) {
		revision := vmcp.Status.ProfileConfigurationRevisions[profileID]
		if len(revision.Superseded) == 0 {
			continue
		}
		var kept []string
		for _, superseded := range revision.Superseded {
			if slices.ContainsFunc(instances, func(instance v1.VMCPInstance) bool {
				return instance.Spec.Manifest.VMCPID == vmcp.Name && instance.Status.AgentConfigurationRevisions[profileID] == superseded
			}) {
				kept = append(kept, superseded)
				continue
			}
			if _, err := s.credentials.DeleteCredential(ctx, ProfileConfigurationCredentialContext(vmcp.Name, profileID), superseded); err != nil {
				return changed, fmt.Errorf("delete superseded configuration revision %q of vMCP %q profile %q: %w", superseded, vmcp.Name, profileID, err)
			}
			slog.Info("Deleted superseded vMCP profile configuration revision", "vmcp", vmcp.Name, "profile", profileID, "revision", superseded)
			changed = true
		}
		revision.Superseded = kept
		setProfileConfigurationRevision(vmcp, profileID, revision)
	}
	return changed, nil
}

// ProfileConfigurableFields returns the configuration of a component that a profile can set for agents: the values a
// user would enter on their own connection (the userAllowed policy), less those the catalog entry supplies itself, as
// a literal (Static) or from a Secret (SecretBinding). Fixed and prohibited values never are, so a profile never
// changes what the vMCP fixes for everyone. Keys are unique, in the order of the component's configuration.
func ProfileConfigurableFields(component types.VMCPComponent) []types.MCPConfig {
	var fields []types.MCPConfig
	for _, config := range ComponentConfig(component) {
		if config.UserAllowed && !config.Static && config.SecretBinding == nil &&
			!slices.ContainsFunc(fields, func(field types.MCPConfig) bool { return field.Key == config.Key }) {
			fields = append(fields, config)
		}
	}
	return fields
}

// ValidateProfileConfiguration checks a change to a profile's configuration against components, the vMCP's components
// as ComponentsForInstance returns them. Every key must be one a profile can set (ProfileConfigurableFields), and a URL
// must have the hostname a hostname-constrained remote requires, as for a user's own connection
// (ValidateAndEncodeUserConfiguration). It returns the values to set and the keys to clear, keyed by ConfigurationKey.
func ValidateProfileConfiguration(components []types.VMCPComponent, update map[string]map[string]*string) (map[string]string, []string, error) {
	byID := make(map[string]types.VMCPComponent, len(components))
	for _, component := range components {
		byID[component.ID] = component
	}
	values := types.VMCPConfiguration{Components: map[string]map[string]string{}}
	var cleared []string
	for componentID, changes := range update {
		component, ok := byID[componentID]
		if !ok {
			return nil, nil, types.NewErrBadRequest("unknown component ID %q", componentID)
		}
		for key, value := range changes {
			if err := checkProfileConfigurable(component, key); err != nil {
				return nil, nil, err
			}
			if value == nil || strings.TrimSpace(*value) == "" {
				cleared = append(cleared, ConfigurationKey(componentID, key))
				continue
			}
			if values.Components[componentID] == nil {
				values.Components[componentID] = map[string]string{}
			}
			values.Components[componentID][key] = *value
		}
	}
	set, err := ValidateAndEncodeUserConfiguration(types.VMCPManifest{Components: components}, values)
	if err != nil {
		return nil, nil, types.NewErrBadRequest("invalid profile configuration: %v", err)
	}
	return set, cleared, nil
}

// checkProfileConfigurable refuses a key that a profile cannot set on component, saying why.
func checkProfileConfigurable(component types.VMCPComponent, key string) error {
	var policy types.VMCPConfigurationPolicyType
	for _, candidate := range component.Configuration {
		if candidate.Key == key {
			policy = candidate.Policy
			break
		}
	}
	configs := ComponentConfig(component)
	index := slices.IndexFunc(configs, func(config types.MCPConfig) bool { return config.Key == key })
	switch {
	case policy == types.VMCPConfigurationPolicyFixed:
		return types.NewErrBadRequest("configuration %q of component %q is fixed by the vMCP for everyone, agents included; a profile cannot set it", key, component.ID)
	case policy != types.VMCPConfigurationPolicyUserAllowed:
		return types.NewErrBadRequest("configuration %q of component %q is not user-allowed; a profile can only set values a user would enter", key, component.ID)
	case index < 0:
		return types.NewErrBadRequest("component %q has no configuration %q", component.ID, key)
	case configs[index].Static:
		return types.NewErrBadRequest("configuration %q of component %q is set by its catalog entry; a profile cannot set it", key, component.ID)
	case configs[index].SecretBinding != nil:
		return types.NewErrBadRequest("configuration %q of component %q is bound to a Secret; a profile cannot set it", key, component.ID)
	}
	return nil
}

// configurableValues returns the values among values, keyed by ConfigurationKey, that a profile can still set on
// components.
func configurableValues(components []types.VMCPComponent, values map[string]string) map[string]string {
	result := map[string]string{}
	for _, component := range components {
		for _, field := range ProfileConfigurableFields(component) {
			key := ConfigurationKey(component.ID, field.Key)
			if value, ok := values[key]; ok && value != "" {
				result[key] = value
			}
		}
	}
	return result
}

// ProfileStatus reports which configuration keys a profile of vmcp sets, for every component the profile grants
// (ProfileGrantsComponent), from the keys its current revision publishes, and whether superseded revisions are still to
// be collected. It describes the profile alone, and never reads or returns a value. Values the profile kept for a
// component it no longer grants are not reported: they are not used.
func ProfileStatus(vmcp v1.VMCP, profile types.VMCPProfile) types.VMCPProfileStatus {
	revision := vmcp.Status.ProfileConfigurationRevisions[profile.ID]
	set := revision.Keys
	status := types.VMCPProfileStatus{
		ProfileID:         profile.ID,
		AgentCapable:      profile.IsAgentCapable(),
		Components:        []types.VMCPProfileComponentStatus{},
		PendingCollection: len(revision.Superseded) > 0,
	}
	for _, component := range ComponentsForInstance(vmcp, v1.VMCPInstance{}) {
		if !ProfileGrantsComponent(profile, component.ID) {
			continue
		}
		configuration := types.VMCPProfileConfigurationStatus{Set: []string{}, MissingRequired: []string{}, OptionalUnset: []string{}}
		for _, field := range ProfileConfigurableFields(component) {
			switch {
			case slices.Contains(set, ConfigurationKey(component.ID, field.Key)):
				configuration.Set = append(configuration.Set, field.Key)
			case field.Required:
				configuration.MissingRequired = append(configuration.MissingRequired, field.Key)
			default:
				configuration.OptionalUnset = append(configuration.OptionalUnset, field.Key)
			}
		}
		status.Components = append(status.Components, types.VMCPProfileComponentStatus{ComponentID: component.ID, Configuration: configuration})
	}
	return status
}
