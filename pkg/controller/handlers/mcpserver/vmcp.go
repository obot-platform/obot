package mcpserver

import (
	"errors"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// SyncVMCPConfiguration copies the configuration for a VMCP component into
// the backward-compatible credential consumed by its MCPServer.
func (h *Handler) SyncVMCPConfiguration(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.VMCPInstanceID == "" {
		return nil
	}

	var instance v1.VMCPInstance
	if err := req.Get(&instance, server.Namespace, server.Spec.VMCPInstanceID); apierrors.IsNotFound(err) {
		// The cleanup handler removes servers whose VMCP instance no longer exists.
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP instance %q: %w", server.Spec.VMCPInstanceID, err)
	}
	if server.Spec.UserID != instance.Spec.UserID {
		return fmt.Errorf("MCPServer %q user %q does not match VMCP instance user %q", server.Name, server.Spec.UserID, instance.Spec.UserID)
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, server.Namespace, instance.Spec.Manifest.VMCPID); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP %q: %w", instance.Spec.Manifest.VMCPID, err)
	}

	if server.Status.VMCPStaticConfigurationHash == vmcp.Spec.StaticConfigurationHash &&
		server.Status.VMCPUserConfigurationHash == instance.Status.UserConfigurationHash {
		return nil
	}

	component, ok := vmcpComponent(vmcp.Spec.Manifest, server.Spec.VMCPComponentID)
	if !ok {
		return fmt.Errorf("VMCP %q does not contain component %q", vmcp.Name, server.Spec.VMCPComponentID)
	}

	staticConfiguration, err := h.revealVMCPConfiguration(
		req,
		vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
	)
	if err != nil {
		return fmt.Errorf("reveal static configuration for VMCP %q: %w", vmcp.Name, err)
	}
	userConfiguration, err := h.revealVMCPConfiguration(
		req,
		vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
	)
	if err != nil {
		return fmt.Errorf("reveal user configuration for VMCP instance %q: %w", instance.Name, err)
	}

	if err := h.gatewayClient.UpsertCredential(req.Ctx, gatewaytypes.Credential{
		Context: fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name),
		Name:    server.Name,
		Secrets: mcpServerConfiguration(*component, staticConfiguration, userConfiguration),
	}); err != nil {
		return fmt.Errorf("store configuration credential for MCPServer %q: %w", server.Name, err)
	}

	server.Status.VMCPStaticConfigurationHash = vmcp.Spec.StaticConfigurationHash
	server.Status.VMCPUserConfigurationHash = instance.Status.UserConfigurationHash
	if err := req.Client.Status().Update(req.Ctx, server); err != nil {
		return fmt.Errorf("update configuration hashes for MCPServer %q: %w", server.Name, err)
	}
	return nil
}

func (h *Handler) revealVMCPConfiguration(req router.Request, credentialContext string) (map[string]string, error) {
	credential, err := h.gatewayClient.RevealCredential(req.Ctx,
		[]string{credentialContext},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err == nil {
		return credential.Secrets, nil
	}
	if errors.As(err, &client.CredentialNotFoundError{}) {
		return map[string]string{}, nil
	}
	return nil, err
}

func vmcpComponent(manifest types.VMCPManifest, componentID string) (*types.VMCPComponent, bool) {
	for index := range manifest.Components {
		if manifest.Components[index].ID == componentID {
			return &manifest.Components[index], true
		}
	}
	return nil, false
}

func mcpServerConfiguration(component types.VMCPComponent, staticConfiguration, userConfiguration map[string]string) map[string]string {
	configuration := make(map[string]string)
	for _, policy := range component.Configuration {
		var source map[string]string
		switch policy.Policy {
		case types.VMCPConfigurationPolicyFixed:
			source = staticConfiguration
		case types.VMCPConfigurationPolicyUserAllowed:
			source = userConfiguration
		default:
			continue
		}

		if value, ok := source[vmcpconfig.ConfigurationKey(component.ID, policy.Key)]; ok {
			configuration[policy.Key] = value
		}
	}
	return configuration
}
