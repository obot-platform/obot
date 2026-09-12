package mcpserver

import (
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// SyncVMCPConfiguration copies the configuration for a VMCP component into
// the configuration credential consumed by its MCPServer.
func (h *Handler) SyncVMCPConfiguration(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.VMCPInstanceID == "" && server.Spec.VMCPID == "" {
		return nil
	}

	var instance v1.VMCPInstance
	vmcpID := server.Spec.VMCPID
	if server.Spec.VMCPInstanceID != "" {
		if err := req.Get(&instance, server.Namespace, server.Spec.VMCPInstanceID); apierrors.IsNotFound(err) {
			// The cleanup handler removes servers whose VMCP instance no longer exists.
			return nil
		} else if err != nil {
			return fmt.Errorf("get VMCP instance %q: %w", server.Spec.VMCPInstanceID, err)
		}
		if server.Spec.UserID != instance.Spec.UserID {
			return fmt.Errorf("MCPServer %q user %q does not match VMCP instance user %q", server.Name, server.Spec.UserID, instance.Spec.UserID)
		}
		vmcpID = instance.Spec.Manifest.VMCPID
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, server.Namespace, vmcpID); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP %q: %w", vmcpID, err)
	}
	shared := vmcpconfig.IsMultiUser(vmcp.Spec.Manifest)
	if shared != (server.Spec.VMCPInstanceID == "") {
		// The owning controller deletes servers from the previous sharing mode.
		return nil
	}

	// The VMCP controllers rebuild Spec.Manifest from the component's catalog entry snapshot
	// without touching either configuration hash, which drops any URL resolved from a template.
	// Tracking the snapshot digest catches that rebuild without revealing a credential to find out.
	snapshotHash := server.Annotations[v1.VMCPSnapshotDigestAnnotation]
	if server.Status.VMCPStaticConfigurationHash == vmcp.Spec.StaticConfigurationHash &&
		server.Status.VMCPUserConfigurationHash == instance.Status.UserConfigurationHash &&
		server.Status.VMCPSnapshotHash == snapshotHash {
		return nil
	}

	effective := vmcp.Spec.Manifest
	effective.Components = vmcpconfig.ComponentsForInstance(vmcp, instance)
	component, ok := vmcpComponent(effective, server.Spec.VMCPComponentID)
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
	var userConfiguration map[string]string
	if !shared {
		userConfiguration, err = h.revealVMCPConfiguration(
			req,
			vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		)
		if err != nil {
			return fmt.Errorf("reveal user configuration for VMCP instance %q: %w", instance.Name, err)
		}
	}

	configuration := mcpServerConfiguration(*component, staticConfiguration, userConfiguration)
	if err := h.gatewayClient.UpsertCredential(req.Ctx, gatewaytypes.Credential{
		Context: server.CredentialContext(server.Spec.UserID),
		Name:    server.Name,
		Secrets: configuration,
	}); err != nil {
		return fmt.Errorf("store configuration credential for MCPServer %q: %w", server.Name, err)
	}

	// Update the spec before the status: the client writes the new resource version back onto
	// the object, so the status update below still applies cleanly.
	if url, owned := resolvedComponentURL(*component, server, configuration); owned && server.Spec.Manifest.RemoteConfig.URL != url {
		server.Spec.Manifest.RemoteConfig.URL = url
		if err := req.Client.Update(req.Ctx, server); err != nil {
			return fmt.Errorf("store resolved URL for MCPServer %q: %w", server.Name, err)
		}
	}

	server.Status.VMCPStaticConfigurationHash = vmcp.Spec.StaticConfigurationHash
	server.Status.VMCPUserConfigurationHash = instance.Status.UserConfigurationHash
	server.Status.VMCPSnapshotHash = snapshotHash
	if err := req.Client.Status().Update(req.Ctx, server); err != nil {
		return fmt.Errorf("update configuration hashes for MCPServer %q: %w", server.Name, err)
	}
	return nil
}

// resolvedComponentURL expands the remote URL template on a component server. The second
// return reports whether this controller maintains the server's URL field; a URL template is
// the only case it does. When it does, the returned URL is the value the field must hold, and
// an empty one means the template cannot be resolved here and any URL already stored has to be
// cleared. Leaving a stale URL behind would keep the runtime from expanding the template, since
// it only does so for a server whose URL is empty.
//
// A shared component server backs every user of a multi-user VMCP, and user-provided values are
// resolved per request rather than stored (see SessionManager.sharedVMCPConfiguration). A
// template referencing one of those has no single answer, so it is left to the runtime.
func resolvedComponentURL(component types.VMCPComponent, server *v1.MCPServer, configuration map[string]string) (string, bool) {
	manifest := server.Spec.Manifest
	if manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil || manifest.RemoteConfig.URLTemplate == "" {
		return "", false
	}

	if server.Spec.VMCPInstanceID == "" {
		references := mcp.URLTemplateReferences(manifest.RemoteConfig.URLTemplate)
		for _, policy := range component.Configuration {
			if policy.Policy == types.VMCPConfigurationPolicyUserAllowed && slices.Contains(references, policy.Key) {
				return "", true
			}
		}
	}

	url, missing := mcp.ResolveRemoteURLTemplate(manifest, configuration)
	if len(missing) > 0 {
		return "", true
	}
	return url, true
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

func (h *Handler) syncVMCPOAuthCredentialStatus(req router.Request, server *v1.MCPServer) error {
	ref, sourceID, err := vmcpconfig.ServerOAuthCredentialReference(req.Ctx, req.Client, *server)
	if err != nil {
		return err
	}
	checkHash, err := vmcpconfig.OAuthCredentialCheckHash(req, server.Namespace, ref, sourceID)
	if err != nil {
		return err
	}
	if server.Status.OAuthCredentialCheckHash == checkHash {
		return nil
	}
	var configured bool
	if ref != "" {
		_, err := h.gatewayClient.RevealCredential(req.Ctx, []string{ref}, system.StaticOAuthCredentialName)
		if err != nil && !errors.As(err, &client.CredentialNotFoundError{}) {
			return err
		}
		configured = err == nil
	}
	server.Status.OAuthCredentialCheckHash = checkHash
	server.Status.OAuthCredentialConfigured = configured
	return req.Client.Status().Update(req.Ctx, server)
}
