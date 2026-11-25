package vmcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DelegatedSetupGateway is the storage of the upstream OAuth tokens and pending authorizations of what users set up
// through agents. The gateway client implements it.
type DelegatedSetupGateway interface {
	DeleteDelegatedOwnerMCPOAuthState(ctx context.Context, match func(principal.DelegatedOwner) bool) error
}

// DelegatedSetupCleanup removes what users set up through agents (agentdelegation.Cleanup): the pairs' instances, and
// with them, through the instances' finalizers and DeleteRefs, their configuration credentials and their component
// servers and connections, whose finalizers delete the credentials and upstream OAuth tokens keyed by them. The pairs'
// upstream OAuth tokens and pending authorizations are also deleted at once, by owner, so that none outlives a revoked
// delegation, even one stored for a connection already gone.
type DelegatedSetupCleanup struct {
	storage kclient.Client
	gateway DelegatedSetupGateway
}

func NewDelegatedSetupCleanup(storage kclient.Client, gateway DelegatedSetupGateway) *DelegatedSetupCleanup {
	return &DelegatedSetupCleanup{
		storage: storage,
		gateway: gateway,
	}
}

// CleanupDelegations removes the setup of the pairs that scope names. It is idempotent, and removes setup whether or
// not a delegation exists for it.
func (c *DelegatedSetupCleanup) CleanupDelegations(ctx context.Context, scope agentdelegation.Scope) error {
	if scope.AgentID == 0 && scope.UserID == 0 {
		return fmt.Errorf("a cleanup of what users set up through agents must name an agent, a user or both")
	}
	match := func(pair principal.DelegatedOwner) bool {
		return (scope.AgentID == 0 || pair.AgentID == scope.AgentID) && (scope.UserID == 0 || pair.UserID == scope.UserID)
	}

	var instances v1.VMCPInstanceList
	listOptions := []kclient.ListOption{kclient.InNamespace(system.DefaultNamespace)}
	if scope.AgentID != 0 && scope.UserID != 0 {
		// One pair: its instances are found by owner.
		listOptions = append(listOptions, kclient.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.userID", principal.DelegatedOwner(scope).String())})
	}
	if err := c.storage.List(ctx, &instances, listOptions...); err != nil {
		return fmt.Errorf("list vMCP instances: %w", err)
	}
	var deleted int
	for i := range instances.Items {
		if pair, ok := PairOwner(instances.Items[i]); !ok || !match(pair) {
			continue
		}
		if err := kclient.IgnoreNotFound(c.storage.Delete(ctx, &instances.Items[i])); err != nil {
			return fmt.Errorf("delete vMCP instance %q: %w", instances.Items[i].Name, err)
		}
		deleted++
	}

	if err := c.gateway.DeleteDelegatedOwnerMCPOAuthState(ctx, match); err != nil {
		return fmt.Errorf("delete upstream OAuth tokens set up through agents: %w", err)
	}
	slog.Info("Removed what users set up through agents", "agentID", scope.AgentID, "userID", scope.UserID, "vmcpInstances", deleted)
	return nil
}

// DeletePairInstances deletes the instances of what users set up for vmcp through agents, as the vMCP is deleted, so
// that, as for a user's own setup, nothing anyone set up for it outlives it. Deleting an instance removes its
// configuration credential, component servers and connections, and their upstream OAuth tokens. Deleting what is
// already gone succeeds.
func DeletePairInstances(ctx context.Context, client kclient.Client, vmcp v1.VMCP) error {
	var instances v1.VMCPInstanceList
	if err := client.List(ctx, &instances, kclient.InNamespace(vmcp.Namespace), kclient.MatchingFields{"spec.manifest.vmcpID": vmcp.Name}); err != nil {
		return fmt.Errorf("list the instances of vMCP %q: %w", vmcp.Name, err)
	}
	for i := range instances.Items {
		if _, ok := PairOwner(instances.Items[i]); !ok {
			continue
		}
		if err := kclient.IgnoreNotFound(client.Delete(ctx, &instances.Items[i])); err != nil {
			return fmt.Errorf("delete vMCP instance %q: %w", instances.Items[i].Name, err)
		}
	}
	return nil
}
