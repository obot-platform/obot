package vmcp

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SyncStatus reports definition readiness without depending on individual users'
// configuration. Shared runtime status is watched through its component servers.
func (h *Handler) SyncStatus(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)

	credential, err := h.revealCredential(req.Ctx, []string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)}, vmcpconfig.ConfigurationCredentialName())
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return err
	}
	shared := vmcpconfig.IsMultiUser(vmcp.Spec.Manifest)
	var servers v1.MCPServerList
	if shared {
		if err := req.List(&servers, &kclient.ListOptions{Namespace: vmcp.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.vmcpID", vmcp.Name)}); err != nil {
			return err
		}
	}
	statuses := make([]v1.VMCPComponentStatus, 0, len(vmcp.Spec.Manifest.Components))
	ready := len(vmcp.Spec.Manifest.Components) > 0
	for _, component := range vmcp.Spec.Manifest.Components {
		status := v1.VMCPComponentStatus{Name: component.Name}
		for _, previous := range vmcp.Status.Components {
			if previous.Name == component.Name {
				status = previous
				break
			}
		}
		status.Ready, status.Error = true, ""
		if _, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true); err != nil {
			status.Error = fmt.Sprintf("invalid component definition: %v", err)
		} else if missing := vmcpconfig.MissingRequiredConfiguration(component, credential.Secrets, false); len(missing) > 0 {
			status.Error = "missing required administrator configuration: " + strings.Join(missing, ", ")
		}
		if ref := vmcpconfig.ComponentOAuthCredentialReference(component); status.Error == "" && ref != "" {
			// Watch source credential changes, while retaining support for deleted sources.
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, vmcp.Namespace, component.MCPServerCatalogEntryID); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if _, err := h.revealCredential(req.Ctx, []string{ref}, system.StaticOAuthCredentialName); errors.As(err, &gateway.CredentialNotFoundError{}) {
				status.Error = "static OAuth credentials are not configured"
			} else if err != nil {
				return err
			}
		}
		if status.Error == "" && shared {
			status.Error = "waiting for component server"
			for _, server := range servers.Items {
				if server.Spec.VMCPComponentID != component.ID {
					continue
				}
				status.Error = componentServerError(server, component, vmcp.Spec.StaticConfigurationHash)
				break
			}
		}
		status.Ready = status.Error == ""
		ready = ready && status.Ready
		statuses = append(statuses, status)
	}
	if ready == vmcp.Status.Ready && slices.Equal(statuses, vmcp.Status.Components) {
		return nil
	}
	vmcp.Status.Ready, vmcp.Status.Components = ready, statuses
	return req.Client.Status().Update(req.Ctx, vmcp)
}

func componentServerError(server v1.MCPServer, component types.VMCPComponent, staticHash string) string {
	if !server.DeletionTimestamp.IsZero() {
		return "component server is being deleted"
	}
	if server.Annotations[v1.VMCPSnapshotDigestAnnotation] != utils.Digest(component.CatalogEntry) || server.Status.VMCPStaticConfigurationHash != staticHash {
		return "waiting for component configuration"
	}
	if vmcpconfig.ComponentOAuthCredentialReference(component) != "" && !server.Status.OAuthCredentialConfigured {
		return "static OAuth credentials are not configured"
	}
	switch server.Status.DeploymentStatus {
	case "", "Available", "Shutdown":
		// Servers are launched on demand; an idle server is still ready to connect.
		return ""
	default:
		return "component deployment: " + server.Status.DeploymentStatus
	}
}
