package vmcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

// The vMCP controller probes the remote components that agents reach, without any credential, to learn whether they
// ask for OAuth, and records what it finds in the vMCP's status (v1.VMCPStatus.ComponentOAuth). That is how a
// component that asks for OAuth, but whose catalog entry does not require static OAuth, is known to need it before
// any profile holds a grant for it (requiresOAuth). Agent readiness, profile status and the connect path only
// read the record; none of them probe.

// ComponentOAuthURLDigest is the digest that a probe of a component of a vMCP at url is recorded under, so that the
// status of the vMCP never holds the URL, which may come from configuration.
func ComponentOAuthURLDigest(vmcpID, componentID, url string) string {
	return utils.Digest([]any{"vmcp-component-oauth-url/v1", vmcpID, componentID, url})
}

// ComponentOAuthProbed reports whether a component is probed for OAuth, and so whether what a probe found counts for
// it. Only a remote server can ask for OAuth. One whose catalog entry requires static OAuth is known to need it, so it
// is not probed. Nor is one that takes configured headers: the probe sends none, so a server that answers it with an
// OAuth challenge may still accept a connection that sends them, such as one with an API key, and must not be held to
// an OAuth grant on the probe's word. Such a component is known to need OAuth once a profile holds a grant for it.
func ComponentOAuthProbed(component types.VMCPComponent) bool {
	return ComponentUsesOAuth(component) &&
		ComponentOAuthCredentialReference(component) == "" &&
		!slices.ContainsFunc(component.CatalogEntry.Manifest.Config, func(config types.MCPConfig) bool { return config.Usage == types.Header })
}

// ProbedOAuthRequirement returns what the vMCP controller last found by probing component at url, for the component's
// current definition. It is VMCPComponentOAuthUnknown when it has not probed that URL, when the probe failed, or when
// the component is not probed (ComponentOAuthProbed).
func ProbedOAuthRequirement(vmcp v1.VMCP, component types.VMCPComponent, url string) v1.VMCPComponentOAuthRequirement {
	probes, ok := currentProbes(vmcp, component)
	if !ok || url == "" {
		return v1.VMCPComponentOAuthUnknown
	}
	digest := ComponentOAuthURLDigest(vmcp.Name, component.ID, url)
	for _, probe := range probes {
		if probe.URLDigest == digest {
			return probe.Required
		}
	}
	return v1.VMCPComponentOAuthUnknown
}

// currentProbes returns the probes the vMCP controller recorded for component, and whether they count for it: it is
// probed (ComponentOAuthProbed), and they are of its current definition.
func currentProbes(vmcp v1.VMCP, component types.VMCPComponent) ([]v1.VMCPComponentOAuthProbe, bool) {
	record, ok := vmcp.Status.ComponentOAuth[component.ID]
	if !ok || !ComponentOAuthProbed(component) || record.DefinitionHash != ProfileOAuthGrantDefinitionHash(component) {
		return nil, false
	}
	return record.Probes, true
}

// probedRequiredAnywhere reports whether a probe of any URL of component, for its current definition, found that it
// asks for OAuth. Only then can the URL that an agent reaches matter, so the URL, which may need configuration to be
// read, is resolved only then.
func probedRequiredAnywhere(vmcp v1.VMCP, component types.VMCPComponent) bool {
	probes, _ := currentProbes(vmcp, component)
	return slices.ContainsFunc(probes, func(probe v1.VMCPComponentOAuthProbe) bool {
		return probe.Required == v1.VMCPComponentOAuthRequired
	})
}

// probedRequired reports whether the vMCP controller found that component asks for OAuth at the URL that the
// connection of an agent whose profiles apply to it as selection reaches (ResolveComponentTarget). It reads the vMCP's
// status and, for a URL that comes from configuration, only when some probe of the component found that it asks for
// OAuth, the configuration that sets it; it never contacts the component. A URL that cannot be resolved, because a
// value it needs is missing or the configured URL is refused, is not known to need OAuth: configuration readiness
// reports it.
func (s *ProfileOAuthGrantStore) probedRequired(ctx context.Context, vmcp v1.VMCP, selection AgentSelection, component types.VMCPComponent) (bool, error) {
	if !probedRequiredAnywhere(vmcp, component) {
		return false, nil
	}
	target, _, err := ResolveComponentTarget(ctx, s.targets, vmcp, selection, component)
	if err != nil {
		return false, fmt.Errorf("resolve the upstream URL of component %q: %w", component.ID, err)
	}
	return ProbedOAuthRequirement(vmcp, component, target.URL) == v1.VMCPComponentOAuthRequired, nil
}
