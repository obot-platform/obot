package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
)

var (
	_ vmcpaccess.ComponentTargetResolver = (*SessionManager)(nil)
)

// ComponentTarget resolves the upstream URL that a remote component's connection reaches for an agent whose profile is
// profile (vmcpaccess.ComponentTargetResolver). It builds the URL from the same values, merged and expanded by the same
// functions, as the agent's component server is given:
//
//   - The profile's values are resolved as for the agent's instance (vmcpaccess.ResolveComponentConfiguration and
//     ProfileConfigurationStore.AgentConfigurationValues).
//   - They are merged with the vMCP's fixed values as the MCPServer controller merges them for the component server
//     (vmcpaccess.ComponentServerConfiguration).
//   - A hostname-constrained remote reaches the configured __url, which must have the hostname, as the runtime reads it
//     (configureRemoteRuntime); a URL template is expanded with the catalog entry's static values as the controller
//     expands it for the server (ResolveURLTemplate, as resolvedComponentURL calls it).
//
// A component whose definition fixes the URL reaches it whoever connects. Only the values the URL needs are read. A URL
// that does not satisfy its hostname, or values that change the host a URL template pins, are an ErrBadRequest; a value
// it needs that neither the profile nor the vMCP supplies leaves the URL empty, with the keys in Missing.
func (sm *SessionManager) ComponentTarget(ctx context.Context, vmcp v1.VMCP, profile *types.VMCPProfile, component types.VMCPComponent) (vmcpaccess.ComponentTarget, error) {
	if url, ok := vmcpaccess.StaticComponentTargetURL(component); ok {
		return vmcpaccess.ComponentTarget{URL: url}, nil
	}
	remote := component.CatalogEntry.Manifest.RemoteConfig
	if !vmcpaccess.ComponentUsesOAuth(component) || remote == nil {
		return vmcpaccess.ComponentTarget{}, nil
	}
	var keys []string
	switch {
	case remote.Hostname != "":
		keys = []string{"__url"}
	case remote.URLTemplate != "":
		keys = URLTemplateReferences(remote.URLTemplate)
	default:
		return vmcpaccess.ComponentTarget{}, nil
	}

	var target vmcpaccess.ComponentTarget
	resolved := vmcpaccess.ResolveComponentConfiguration(component, profile, vmcp.Status.ProfileConfigurationRevisions)
	needed := vmcpaccess.ComponentConfiguration{ComponentID: component.ID}
	revisions := map[string]string{}
	for _, source := range resolved.Sources {
		if !slices.Contains(keys, source.Field) {
			continue
		}
		needed.Sources = append(needed.Sources, source)
		revisions[source.ProfileID] = source.Revision
		target.Configured = true
	}
	userValues := map[string]string{}
	if len(needed.Sources) > 0 {
		var err error
		userValues, err = vmcpaccess.NewProfileConfigurationStore(sm.gatewayClient).AgentConfigurationValues(ctx, vmcp.Name, vmcpaccess.AgentConfiguration{
			Components: []vmcpaccess.ComponentConfiguration{needed},
			Revisions:  revisions,
		})
		if err != nil {
			return vmcpaccess.ComponentTarget{}, err
		}
	}
	staticValues, err := sm.fixedURLValues(ctx, vmcp, component, keys)
	if err != nil {
		return vmcpaccess.ComponentTarget{}, err
	}
	configuration := vmcpaccess.ComponentServerConfiguration(component, staticValues, userValues)

	var missing []string
	switch {
	case remote.Hostname != "":
		if target.URL = configuration["__url"]; target.URL == "" {
			missing = []string{"__url"}
		} else if err := types.ValidateURLHostname(target.URL, remote.Hostname); err != nil {
			// The configured URL is configuration, so it is not repeated; the hostname is the catalog entry's.
			return vmcpaccess.ComponentTarget{}, types.NewErrBadRequest("the URL configured for component %q does not have hostname %q", component.ID, remote.Hostname)
		}
	default:
		config, err := ResolveStaticConfiguration(ctx, sm.gatewayClient, component.MCPServerCatalogEntryID, component.CatalogEntry.Manifest.StaticConfigurationRevision, component.CatalogEntry.Manifest.Config)
		if err != nil {
			return vmcpaccess.ComponentTarget{}, err
		}
		// As the MCPServer controller expands it for the server (resolvedComponentURL): only a shared vMCP's fixed
		// values, which only admins set, count as text the admin wrote, and values that change the template's host are
		// refused rather than reached.
		var adminKeys []string
		if vmcp.Spec.UserID == "" {
			for _, policy := range component.Configuration {
				if policy.Policy == types.VMCPConfigurationPolicyFixed {
					adminKeys = append(adminKeys, policy.Key)
				}
			}
		}
		target.URL, missing, err = ResolveURLTemplate(remote.URLTemplate, config, configuration, adminKeys)
		if err != nil {
			// The error names the template's variables, never their values.
			return vmcpaccess.ComponentTarget{}, types.NewErrBadRequest("the URL template of component %q: %v", component.ID, err)
		}
	}
	if len(missing) > 0 {
		target.URL, target.Missing = "", missing
	}
	return target, nil
}

// fixedURLValues returns the vMCP's fixed values, keyed by vmcpaccess.ConfigurationKey, when the vMCP fixes any of the
// keys a component's URL needs. It reads nothing otherwise.
func (sm *SessionManager) fixedURLValues(ctx context.Context, vmcp v1.VMCP, component types.VMCPComponent, keys []string) (map[string]string, error) {
	if !slices.ContainsFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
		return policy.Policy == types.VMCPConfigurationPolicyFixed && slices.Contains(keys, policy.Key)
	}) {
		return map[string]string{}, nil
	}
	credential, err := sm.gatewayClient.RevealCredential(ctx, []string{vmcpaccess.StaticConfigurationCredentialContext(vmcp.Name)}, vmcpaccess.StaticConfigurationCredentialName(&vmcp))
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("reveal the fixed configuration of vMCP %q: %w", vmcp.Name, err)
	}
	return credential.Secrets, nil
}
