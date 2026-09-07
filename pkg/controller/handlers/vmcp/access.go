package vmcp

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api/authz"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	userInfo         func(context.Context, uint) (kuser.Info, error)
	acrHelper        *accesscontrolrule.Helper
	revealCredential func(context.Context, []string, string) (gatewaytypes.Credential, error)
}

func New(gatewayClient *gateway.Client, acrHelper *accesscontrolrule.Helper) *Handler {
	return &Handler{userInfo: gatewayClient.UserInfoByID, acrHelper: acrHelper, revealCredential: gatewayClient.RevealCredential}
}

// PruneUnauthorizedComponents preserves missing sources, but removes existing
// entries that the personal vMCP's owner can no longer access.
func (h *Handler) PruneUnauthorizedComponents(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if vmcp.Spec.UserID == "" {
		return nil
	}
	// Register watches, as in the legacy catalog-access cleanup handlers.
	if err := req.List(&v1.AccessControlRuleList{}, &kclient.ListOptions{Namespace: vmcp.Namespace}); err != nil {
		return err
	}
	if err := req.List(&v1.UserGroupChangeList{}, &kclient.ListOptions{Namespace: vmcp.Namespace}); err != nil {
		return err
	}
	if len(vmcp.Spec.Manifest.Components) == 0 {
		return nil
	}
	userID, err := strconv.ParseUint(vmcp.Spec.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid VMCP owner ID %q: %w", vmcp.Spec.UserID, err)
	}
	owner, err := h.userInfo(req.Ctx, uint(userID))
	if err != nil {
		return fmt.Errorf("get VMCP owner %q: %w", vmcp.Spec.UserID, err)
	}
	components := make([]types.VMCPComponent, 0, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, vmcp.Namespace, component.MCPServerCatalogEntryID); apierrors.IsNotFound(err) {
			components = append(components, component)
			continue
		} else if err != nil {
			return err
		}
		allowed, err := authz.UserCanReadCatalogEntry(req.Ctx, owner, &entry, h.acrHelper)
		if err != nil {
			return fmt.Errorf("check VMCP owner access to entry %q: %w", entry.Name, err)
		}
		if allowed {
			components = append(components, component)
		}
	}
	if len(components) == len(vmcp.Spec.Manifest.Components) {
		return nil
	}
	if len(components) == 0 {
		slog.Info("Deleting personal VMCP after catalog access loss", "vmcp", vmcp.Name, "userID", vmcp.Spec.UserID)
		return kclient.IgnoreNotFound(req.Delete(vmcp))
	}
	slog.Info("Pruning personal VMCP after catalog access loss", "vmcp", vmcp.Name, "userID", vmcp.Spec.UserID, "removedComponents", len(vmcp.Spec.Manifest.Components)-len(components))
	previous := vmcp.Spec.Manifest.Components
	vmcp.Spec.Manifest.Components = components
	vmcpconfig.PruneRemovedComponentProfiles(previous, &vmcp.Spec.Manifest)
	return req.Client.Update(req.Ctx, vmcp)
}
