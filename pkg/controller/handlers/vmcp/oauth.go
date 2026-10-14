package vmcp

import (
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SyncOAuthCredentials retries interrupted invalidations and removes orphaned overrides.
func (h *Handler) SyncOAuthCredentials(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	credentials, err := h.credentials.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContextPrefix: vmcpconfig.ComponentOAuthCredentialPrefix(vmcp.Name),
	})
	if err != nil {
		return err
	}
	// Candidates must precede the uncached read: a credential created for a new
	// component must have its already-persisted owner included in this definition.
	var current v1.VMCP
	if err := h.uncachedReader.Get(req.Ctx, kclient.ObjectKeyFromObject(vmcp), &current); err != nil {
		return kclient.IgnoreNotFound(err) // Deletion is handled by the vMCP finalizer.
	}
	if current.UID != vmcp.UID {
		return nil
	}
	*vmcp = current
	allowed := map[string]bool{}
	annotations := map[string]bool{}
	changed := false
	for _, component := range vmcp.Spec.Manifest.Components {
		allowed[vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, component.ID)] = true
		key := vmcpconfig.ComponentOAuthAnnotation(component.ID)
		annotations[key] = true
		if !vmcpconfig.ComponentOAuthPending(*vmcp, component.ID) {
			continue
		}
		if err := vmcpconfig.InvalidateComponentOAuthTokens(req.Ctx, req.Client, h.gatewayClient, *vmcp, component.ID); err != nil {
			return err
		}
		vmcp.Annotations[key] = strings.Replace(vmcp.Annotations[key], "pending:", "ready:", 1)
		changed = true
	}
	for _, credential := range credentials {
		if allowed[credential.Context] {
			continue
		}
		if _, err := h.credentials.DeleteCredential(req.Ctx, credential.Context, credential.Name); err != nil {
			return err
		}
	}
	for key := range vmcp.Annotations {
		if strings.HasPrefix(key, vmcpconfig.ComponentOAuthAnnotationPrefix) && !annotations[key] {
			delete(vmcp.Annotations, key)
			changed = true
		}
	}
	if changed {
		return req.Client.Update(req.Ctx, vmcp)
	}
	return nil
}
