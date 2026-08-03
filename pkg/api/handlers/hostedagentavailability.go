package handlers

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/hostedagentmodels"
	"github.com/obot-platform/obot/pkg/hostedagentrefs"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// agentAvailability reports why an agent cannot be launched on this
// installation, empty when it can.
//
// A template is written once and used everywhere, so it names things that may
// not exist here: a harness that speaks a protocol no configured provider
// serves, or an MCP server that was never installed. Launching it anyway
// produces a sandbox that fails at its first request, with an error the user
// cannot act on. Saying so up front turns that into something an administrator
// can fix.
type agentAvailability struct {
	client    kclient.Client
	namespace string
	harnesses map[string]types.HarnessManifest
}

func newAgentAvailability(ctx context.Context, client kclient.Client, namespace string) (*agentAvailability, error) {
	var harnesses v1.HarnessList
	if err := client.List(ctx, &harnesses, &kclient.ListOptions{Namespace: namespace}); err != nil {
		return nil, fmt.Errorf("failed to list harnesses: %w", err)
	}

	availability := &agentAvailability{
		client:    client,
		namespace: namespace,
		harnesses: make(map[string]types.HarnessManifest, len(harnesses.Items)),
	}
	for _, harness := range harnesses.Items {
		availability.harnesses[harness.Name] = harness.Spec.Manifest
	}
	return availability, nil
}

// reasons returns every reason an agent cannot be launched, so an administrator
// sees the whole list rather than fixing one and being told about the next.
func (a *agentAvailability) reasons(ctx context.Context, manifest types.HostedAgentManifest) []string {
	var reasons []string

	if _, ok := a.harnesses[manifest.HarnessID]; !ok {
		return []string{"its harness is not registered"}
	}

	if reason := a.modelReason(ctx, manifest); reason != "" {
		reasons = append(reasons, reason)
	}
	reasons = append(reasons, a.mcpReasons(ctx, manifest)...)
	reasons = append(reasons, a.skillReasons(ctx, manifest)...)
	return reasons
}

func (a *agentAvailability) modelReason(ctx context.Context, manifest types.HostedAgentManifest) string {
	models, err := hostedagentmodels.Resolve(ctx, a.client, a.namespace, manifest.Models, manifest.ModelProviders)
	if err != nil {
		// A failure to read models is not a statement about the agent, and
		// blocking every agent on a transient error would be worse than
		// letting one through.
		return ""
	}
	if len(models) > 0 {
		return ""
	}

	// Naming the provider is what makes this actionable: an administrator can
	// go and configure it. "No models available" would not say which.
	if len(manifest.ModelProviders) == 1 {
		return fmt.Sprintf("the %s is not configured", manifest.ModelProviders[0])
	}
	if len(manifest.ModelProviders) > 1 {
		return fmt.Sprintf("none of the model providers it needs are configured: %v", manifest.ModelProviders)
	}
	if len(manifest.Models) > 0 {
		return "none of the models it is configured with are available"
	}
	return ""
}

// mcpReasons names the MCP servers the agent refers to that do not exist here.
//
// A reference is resolved against its source; a bare ID is looked up directly.
func (a *agentAvailability) mcpReasons(ctx context.Context, manifest types.HostedAgentManifest) []string {
	var missing []string
	for _, id := range manifest.MCPServers {
		if id == "" || id == "*" {
			continue
		}
		if hostedagentrefs.IsReference(id) {
			if _, err := hostedagentrefs.ResolveMCP(ctx, a.client, a.namespace, id); err != nil {
				missing = append(missing, id)
			}
			continue
		}
		if !a.mcpExists(ctx, id) {
			missing = append(missing, id)
		}
	}
	return describeMissing("MCP server", missing)
}

// skillReasons names the skills the agent refers to that do not exist here.
func (a *agentAvailability) skillReasons(ctx context.Context, manifest types.HostedAgentManifest) []string {
	var missing []string
	for _, id := range manifest.Skills {
		if id == "" {
			continue
		}
		if hostedagentrefs.IsReference(id) {
			if _, err := hostedagentrefs.ResolveSkill(ctx, a.client, a.namespace, id); err != nil {
				missing = append(missing, id)
			}
			continue
		}
		var skill v1.Skill
		if !a.found(a.client.Get(ctx, kclient.ObjectKey{Namespace: a.namespace, Name: id}, &skill)) {
			missing = append(missing, id)
		}
	}
	return describeMissing("skill", missing)
}

// describeMissing phrases the absences so the reason names what to install.
func describeMissing(kind string, missing []string) []string {
	switch len(missing) {
	case 0:
		return nil
	case 1:
		return []string{fmt.Sprintf("the %s %s is not installed", kind, missing[0])}
	default:
		return []string{fmt.Sprintf("%d %ss it uses are not installed: %v", len(missing), kind, missing)}
	}
}

// mcpExists reports whether an MCP gateway ID names something that is here.
//
// The ID is the same handle /mcp-connect accepts, which may be a server, an
// instance of a multi-user server, or a catalog entry -- so each is tried
// rather than inferring the kind from the ID's shape.
func (a *agentAvailability) mcpExists(ctx context.Context, id string) bool {
	key := kclient.ObjectKey{Namespace: a.namespace, Name: id}

	switch {
	case system.IsMCPServerInstanceID(id):
		var instance v1.MCPServerInstance
		return a.found(a.client.Get(ctx, key, &instance))
	case system.IsMCPServerID(id):
		var server v1.MCPServer
		return a.found(a.client.Get(ctx, key, &server))
	}

	var entry v1.MCPServerCatalogEntry
	if a.found(a.client.Get(ctx, key, &entry)) {
		return true
	}
	var server v1.MCPServer
	return a.found(a.client.Get(ctx, key, &server))
}

// found treats anything other than a definite absence as present. A read that
// failed for another reason says nothing about whether the server exists, and
// reporting it missing would block an agent over a transient error.
func (a *agentAvailability) found(err error) bool {
	return !apierrors.IsNotFound(err)
}

// applyTo annotates an agent with why it cannot be launched here.
func (a *agentAvailability) applyTo(ctx context.Context, agent types.HostedAgent) types.HostedAgent {
	agent.UnavailableReasons = a.reasons(ctx, agent.HostedAgentManifest)
	return agent
}
