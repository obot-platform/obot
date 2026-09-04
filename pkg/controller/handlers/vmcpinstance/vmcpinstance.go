package vmcpinstance

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Handler struct {
	revealCredential func(context.Context, []string, string) (gatewaytypes.Credential, error)
}

func New(gatewayClient *gateway.Client) *Handler {
	handler := &Handler{}
	if gatewayClient != nil {
		handler.revealCredential = gatewayClient.RevealCredential
	}
	return handler
}

// SyncUserConfigurationHash records the current instance credential without
// exposing its values. Only values still allowed by the VMCP policy contribute
// to the hash.
func (h *Handler) SyncUserConfigurationHash(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP %q: %w", instance.Spec.Manifest.VMCPID, err)
	}

	configuration := map[string]string{}
	credential, err := h.revealCredential(req.Ctx,
		[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err == nil {
		configuration = userAllowedConfiguration(vmcp.Spec.Manifest, credential.Secrets)
	} else if !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("reveal configuration credential for VMCP instance %q: %w", instance.Name, err)
	}

	configurationHash := utils.Digest(configuration)
	if instance.Status.UserConfigurationHash == configurationHash {
		return nil
	}
	instance.Status.UserConfigurationHash = configurationHash
	return req.Client.Status().Update(req.Ctx, instance)
}

func userAllowedConfiguration(manifest types.VMCPManifest, credential map[string]string) map[string]string {
	configuration := make(map[string]string)
	for _, component := range manifest.Components {
		for _, policy := range component.Configuration {
			if policy.Policy != types.VMCPConfigurationPolicyUserAllowed {
				continue
			}
			key := vmcpconfig.ConfigurationKey(component.ID, policy.Key)
			if value, ok := credential[key]; ok {
				configuration[key] = value
			}
		}
	}
	return configuration
}

// EnsureMCPServers creates one MCPServer from each catalog-entry snapshot on
// the VMCP. Configuration credentials are intentionally not applied here.
func (*Handler) EnsureMCPServers(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); apierrors.IsNotFound(err) {
		// The cleanup handler removes instances whose VMCP no longer exists.
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP %q: %w", instance.Spec.Manifest.VMCPID, err)
	}

	servers := make([]v1.MCPServer, 0, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		server, err := mcpServerForComponent(instance, component)
		if err != nil {
			return fmt.Errorf("build MCPServer for VMCP component %q: %w", component.Name, err)
		}
		servers = append(servers, server)
	}

	for index := range servers {
		server := &servers[index]
		var existing v1.MCPServer
		if err := req.Get(&existing, server.Namespace, server.Name); err == nil {
			if existing.Spec.VMCPInstanceID != instance.Name || existing.Spec.VMCPComponentID != server.Spec.VMCPComponentID {
				return fmt.Errorf("MCPServer %q already exists with different VMCP ownership", server.Name)
			}
			continue
		} else if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get MCPServer %q: %w", server.Name, err)
		}

		if err := req.Client.Create(req.Ctx, server); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("create MCPServer %q: %w", server.Name, err)
		}
	}

	return nil
}

func mcpServerForComponent(instance *v1.VMCPInstance, component types.VMCPComponent) (v1.MCPServer, error) {
	if component.ID == "" {
		return v1.MCPServer{}, fmt.Errorf("component ID is required")
	}

	manifest, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true)
	if err != nil {
		return v1.MCPServer{}, err
	}

	return v1.MCPServer{
		Name:      name.SafeConcatName(system.MCPServerPrefix+instance.Name, component.ID),
		Namespace: instance.Namespace,
		Spec: v1.MCPServerSpec{
			Manifest:         manifest,
			UnsupportedTools: slices.Clone(component.CatalogEntry.UnsupportedTools),
			UserID:           instance.Spec.UserID,
			VMCPInstanceID:   instance.Name,
			VMCPComponentID:  component.ID,
		},
	}, nil
}
