package cleanup

import (
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DeletedAgent handles an AgentIdentityDelete: it removes what a deleted agent owns. It handles an agent the way
// UserCleanup handles a user, step for step, wherever the step has an agent equivalent:
//
//   - A user's identities and browser sessions have none: an agent authenticates only with its API keys, which
//     DeleteAgent revokes in the transaction that records the deletion, so none is left to revoke here.
//   - A user's AgentInstances, personal vMCPs and PowerUserWorkspace have none: an agent cannot create them.
//   - A user is removed from AccessControlRules, which cannot name an agent.
//
// As for a deleted user, the deleted agent's subjects stay in vMCP profiles, model access policies, skill access
// rules and guardrail filters, where they match nobody, because the agent can never authenticate again and agent IDs
// are never reused. Its MCP OAuth tokens go with the servers and connections whose deletion removes users' tokens,
// and pending authorizations and challenges expire as users' do. The preconfigured OAuth grants it used are the
// vMCPs', keyed by them, and are kept. Audit, usage and budget usage rows are history and
// are kept, and the soft-deleted agent row keeps them resolvable.
//
// Every step is idempotent, and the event object is deleted only after every step succeeded, so a handler that fails
// at any point is retried and finishes what an earlier attempt left.
func DeletedAgent(req router.Request, _ router.Response) error {
	agentDelete := req.Object.(*v1.AgentIdentityDelete)
	agentID := agentDelete.Spec.AgentID
	// Owned resources record their owner as the encoded principal, so this never selects those of the user with the
	// agent's numeric ID.
	byOwner := &kclient.ListOptions{
		Namespace:     req.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", principal.Agent(agentID).String()),
	}
	slog.Info("Starting agent cleanup", "agentID", agentID)

	// Delete vMCPInstances. vmcpinstance.DeleteUnauthorized also deletes a deleted agent's instances; deleting them
	// here as well means the cleanup is complete when this object is deleted. Deleting an instance cascades through
	// DeleteRefs to its dedicated MCPServers and its MCPServerInstances, whose finalizers delete their credentials and
	// MCP OAuth tokens.
	var vmcpInstances v1.VMCPInstanceList
	if err := req.List(&vmcpInstances, byOwner); err != nil {
		return err
	}

	var deletedvMCPInstances int
	for _, instance := range vmcpInstances.Items {
		if err := kclient.IgnoreNotFound(req.Delete(&instance)); err != nil {
			return err
		}

		deletedvMCPInstances++
	}
	slog.Info("Deleted vMCP instances during agent cleanup", "agentID", agentID, "vMCPInstances", deletedvMCPInstances)

	var servers v1.MCPServerList
	if err := req.List(&servers, byOwner); err != nil {
		return err
	}

	var deletedServers int
	for _, server := range servers.Items {
		// Skip multi-user servers in the default MCPCatalog, and servers associated to vMCP components, as
		// UserCleanup does. The latter are deleted with the vMCP instances above.
		if server.Spec.MCPCatalogID == system.DefaultCatalog || server.Spec.VMCPComponentID != "" {
			continue
		}
		if err := req.Delete(&server); err != nil {
			return err
		}
		deletedServers++
	}
	slog.Info("Deleted MCP servers during agent cleanup", "agentID", agentID, "servers", deletedServers, "skipped", len(servers.Items)-deletedServers)

	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, byOwner); err != nil {
		return err
	}

	var instancesDeleted int
	for _, instance := range instances.Items {
		// Instances associated to vMCP components are deleted with their vMCP instance.
		if instance.Spec.VMCPComponentID != "" {
			continue
		}

		if err := req.Delete(&instance); err != nil {
			return err
		}
		instancesDeleted++
	}
	slog.Info("Deleted MCP server instances during agent cleanup", "agentID", agentID, "instances", instancesDeleted)

	// If everything is cleaned up successfully, then delete this object because we don't need it.
	slog.Info("Completed agent cleanup", "agentID", agentID)
	return req.Delete(agentDelete)
}
