package vmcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api/authz"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	userInfo    func(context.Context, principal.Principal) (kuser.Info, error)
	acrHelper   *accesscontrolrule.Helper
	credentials credentialClient
	profileData vmcpconfig.ProfileDataStore
}

type credentialClient interface {
	RevealCredential(ctx context.Context, contexts []string, name string) (gatewaytypes.Credential, error)
	ListCredentials(ctx context.Context, opts gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error)
	DeleteCredential(ctx context.Context, credentialContext, name string) (bool, error)
}

func New(gatewayClient *gateway.Client, acrHelper *accesscontrolrule.Helper, profileData vmcpconfig.ProfileDataStore) *Handler {
	return &Handler{userInfo: gatewayClient.IdentityInfo, acrHelper: acrHelper, credentials: gatewayClient, profileData: profileData}
}

// PruneUnauthorizedComponents preserves missing sources, but removes existing
// entries that the personal vMCP's owner can no longer access.
func (h *Handler) PruneUnauthorizedComponents(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if vmcp.Spec.UserID == "" {
		return nil
	}

	// Register watches, as in the legacy catalog-access cleanup handlers.
	if err := req.List(&v1.AccessControlRuleList{}, &kclient.ListOptions{
		Namespace: vmcp.Namespace,
	}); err != nil {
		return err
	}

	if err := req.List(&v1.UserGroupChangeList{}, &kclient.ListOptions{
		Namespace:     vmcp.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", vmcp.Spec.UserID),
	}); err != nil {
		return err
	}

	if err := req.List(&v1.UserRoleChangeList{}, &kclient.ListOptions{
		Namespace:     vmcp.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", vmcp.Spec.UserID),
	}); err != nil {
		return err
	}

	if len(vmcp.Spec.Manifest.Components) == 0 {
		return nil
	}

	ownerID, err := principal.Parse(vmcp.Spec.UserID)
	if err != nil {
		return fmt.Errorf("invalid VMCP owner ID %q: %w", vmcp.Spec.UserID, err)
	}
	if _, ok := ownerID.UserID(); !ok {
		return fmt.Errorf("invalid VMCP owner ID %q: personal VMCPs are owned by users", vmcp.Spec.UserID)
	}

	owner, err := h.userInfo(req.Ctx, ownerID)
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

	slog.Info("Pruning personal VMCP after catalog access loss", "vmcp", vmcp.Name, "userID", vmcp.Spec.UserID, "removedComponents",
		len(vmcp.Spec.Manifest.Components)-len(components))

	previous := vmcp.Spec.Manifest.Components
	vmcp.Spec.Manifest.Components = components
	vmcpconfig.PruneRemovedComponentProfiles(previous, &vmcp.Spec.Manifest)
	return req.Client.Update(req.Ctx, vmcp)
}
