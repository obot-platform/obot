package vmcp

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

// The vMCP controller probes the remote components that agents reach, without any credential, to learn whether they
// ask for OAuth, and records what it finds in the vMCP's status (v1.VMCPStatus.ComponentOAuth). That is how a
// component that asks for OAuth, but whose catalog entry does not require static OAuth, is known to need it before
// the vMCP holds a grant for it (requiresOAuth). Agent readiness, the vMCP's agent setup and the connect path only
// read the record; none of them probe.

// ComponentOAuthTargetHash identifies what a probe of a component of vmcp is for, and so whether a recorded probe
// still counts for it (v1.VMCPComponentOAuth.TargetHash): the component's definition
// (AgentOAuthGrantDefinitionHash), the catalog entry's static values, and the values the vMCP fixes for the
// component, from which a URL that the definition does not fix is built. A change to any of them can change the URL,
// so the component is probed again. It digests the fixed values' digest, never a value or the URL.
func ComponentOAuthTargetHash(vmcp v1.VMCP, component types.VMCPComponent) string {
	return utils.Digest([]any{
		"vmcp-component-oauth-target/v1",
		AgentOAuthGrantDefinitionHash(component),
		component.CatalogEntry.Manifest.StaticConfigurationRevision,
		vmcp.Spec.ComponentStaticConfigurationHashes[component.ID],
	})
}

// ComponentOAuthProbed reports whether a component is probed for OAuth, and so whether what a probe found counts for
// it. Only a remote server can ask for OAuth. One whose catalog entry requires static OAuth is known to need it, so it
// is not probed. Nor is one that takes configured headers: the probe sends none, so a server that answers it with an
// OAuth challenge may still accept a connection that sends them, such as one with an API key, and must not be held to
// an OAuth grant on the probe's word. Such a component is known to need OAuth once the vMCP holds a grant for it.
func ComponentOAuthProbed(component types.VMCPComponent) bool {
	return ComponentUsesOAuth(component) &&
		ComponentOAuthCredentialReference(component) == "" &&
		!slices.ContainsFunc(component.CatalogEntry.Manifest.Config, func(config types.MCPConfig) bool { return config.Usage == types.Header })
}

// ProbedOAuthRequirement returns what the vMCP controller last found by probing component, for its current target
// (ComponentOAuthTargetHash). It is VMCPComponentOAuthUnknown when it has not probed it, when the probe failed, or when
// the component is not probed (ComponentOAuthProbed). It reads the vMCP's status, and never contacts the component.
func ProbedOAuthRequirement(vmcp v1.VMCP, component types.VMCPComponent) v1.VMCPComponentOAuthRequirement {
	record, ok := vmcp.Status.ComponentOAuth[component.ID]
	if !ok || !ComponentOAuthProbed(component) || record.TargetHash != ComponentOAuthTargetHash(vmcp, component) {
		return v1.VMCPComponentOAuthUnknown
	}
	return record.Required
}
