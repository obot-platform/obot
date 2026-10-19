package handlers

import (
	"errors"
	"strings"
	"uuid"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

func vmcpOAuthComponent(req api.Context) (*v1.VMCP, types.VMCPComponent, error) {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return nil, types.VMCPComponent{}, err
	}

	if vmcp.Spec.SourceURL != "" {
		return nil, types.VMCPComponent{}, types.NewErrBadRequest("component OAuth configuration is not supported for Git-managed vMCPs")
	}

	for _, component := range vmcp.Spec.Manifest.Components {
		if component.ID == req.PathValue("component_id") {
			if component.CatalogEntry.Manifest.Runtime != types.RuntimeRemote {
				return nil, component, types.NewErrBadRequest("static OAuth is only supported for remote components")
			}

			return &vmcp, component, nil
		}
	}
	return nil, types.VMCPComponent{}, types.NewErrNotFound("vMCP component not found")
}

func writeComponentOAuthStatus(req api.Context, vmcp *v1.VMCP, component types.VMCPComponent) error {
	contexts := vmcpconfig.ComponentOAuthCredentialContexts(vmcp.Name, component)
	cred, err := req.GatewayClient.RevealCredential(req.Context(), contexts, system.StaticOAuthCredentialName)
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return req.Write(types.MCPServerOAuthCredentialStatus{Source: "none"})
	}
	if err != nil {
		return err
	}

	source := "catalog"
	if cred.Context == contexts[0] {
		source = "component"
	}
	return req.Write(types.MCPServerOAuthCredentialStatus{Configured: true, ClientID: cred.Secrets["CLIENT_ID"], Source: source})
}

func (*VMCPHandler) GetOAuthCredentials(req api.Context) error {
	vmcp, component, err := vmcpOAuthComponent(req)
	if err != nil {
		return err
	}
	return writeComponentOAuthStatus(req, vmcp, component)
}

func (*VMCPHandler) SetOAuthCredentials(req api.Context) error {
	vmcp, component, err := vmcpOAuthComponent(req)
	if err != nil {
		return err
	}

	var input types.MCPServerOAuthCredentialRequest
	if err := req.Read(&input); err != nil {
		return err
	}

	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" {
		return types.NewErrBadRequest("clientID is required")
	}

	credentialContext := vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, component.ID)
	if err := markComponentOAuthChange(req, vmcp, component, true); err != nil {
		return err
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: credentialContext,
		Name:    system.StaticOAuthCredentialName,
		Secrets: map[string]string{"CLIENT_ID": clientID, "CLIENT_SECRET": strings.TrimSpace(input.ClientSecret)},
	}); err != nil {
		return err
	}

	if err := finishComponentOAuthChange(req, vmcp, component); err != nil {
		return err
	}
	return writeComponentOAuthStatus(req, vmcp, component)
}

func (*VMCPHandler) DeleteOAuthCredentials(req api.Context) error {
	vmcp, component, err := vmcpOAuthComponent(req)
	if err != nil {
		return err
	}

	ids, err := vmcpconfig.ComponentOAuthTokenIDs(req.Context(), req.Storage, *vmcp, component.ID)
	if err != nil {
		return err
	}

	deleted, deleteErr := req.GatewayClient.DeleteCredentialAndMCPOAuthTokens(req.Context(), vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, component.ID), system.StaticOAuthCredentialName, ids)
	if !deleted {
		if deleteErr != nil {
			return deleteErr
		}
		return writeComponentOAuthStatus(req, vmcp, component)
	}

	// Authorization cleanup has committed with the deletion, even if publishing
	// the revision fails. A no-op retry cannot leave the previous tokens valid.
	finishErr := finishComponentOAuthChange(req, vmcp, component)
	if err := errors.Join(deleteErr, finishErr); err != nil {
		return err
	}
	return writeComponentOAuthStatus(req, vmcp, component)
}

func markComponentOAuthChange(req api.Context, vmcp *v1.VMCP, component types.VMCPComponent, validate bool) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(vmcp, vmcp.Name); err != nil {
			return err
		}

		// Revalidate after fetching the current definition, including component removal.
		if validate && vmcp.Spec.SourceURL != "" {
			return types.NewErrBadRequest("cannot configure a Git-managed vMCP")
		}

		found := false
		for _, current := range vmcp.Spec.Manifest.Components {
			found = found || (current.ID == component.ID && current.MCPServerCatalogEntryID == component.MCPServerCatalogEntryID && current.CatalogEntry.Manifest.Runtime == types.RuntimeRemote)
		}

		if validate && !found {
			return types.NewErrNotFound("remote vMCP component was removed or replaced")
		}

		if vmcp.Annotations == nil {
			vmcp.Annotations = map[string]string{}
		}

		vmcp.Annotations[vmcpconfig.ComponentOAuthAnnotation(component.ID)] = "pending:" + uuid.New().String()

		return req.Update(vmcp)
	})
}

func finishComponentOAuthChange(req api.Context, vmcp *v1.VMCP, component types.VMCPComponent) error {
	// Publish again after the write: a controller may have consumed the earlier signal.
	if err := markComponentOAuthChange(req, vmcp, component, false); err != nil {
		if apierrors.IsNotFound(err) {
			_, cleanupErr := req.GatewayClient.DeleteCredential(req.Context(), vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, component.ID), system.StaticOAuthCredentialName)
			return errors.Join(err, cleanupErr)
		}
		return err
	}

	key := vmcpconfig.ComponentOAuthAnnotation(component.ID)
	revision := vmcp.Annotations[key]

	if err := vmcpconfig.InvalidateComponentOAuthTokens(req.Context(), req.Storage, req.GatewayClient, *vmcp, component.ID); err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(vmcp, vmcp.Name); err != nil {
			return err
		}

		if vmcp.Annotations[key] != revision {
			return nil
		}

		vmcp.Annotations[key] = strings.Replace(revision, "pending:", "ready:", 1)

		return req.Update(vmcp)
	})
}
