package mcpserverinstance

import (
	"errors"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) SyncVMCPConfiguration(req router.Request, _ router.Response) error {
	connection := req.Object.(*v1.MCPServerInstance)
	if connection.Spec.VMCPInstanceID == "" {
		return nil
	}
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.Namespace, connection.Spec.VMCPInstanceID); err != nil {
		return kclient.IgnoreNotFound(err)
	}
	var server v1.MCPServer
	if err := req.Get(&server, req.Namespace, connection.Spec.MCPServerName); err != nil {
		return kclient.IgnoreNotFound(err)
	}
	component, err := vmcpconfig.ServerInstanceComponent(req.Ctx, req.Client, *connection, server)
	if err != nil {
		return err
	}
	// Register a dependency on policy changes as well as instance credential changes.
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.Namespace, instance.Spec.Manifest.VMCPID); err != nil {
		return err
	}
	if vmcpconfig.ConnectionConfigurationSynced(*connection, component, instance) {
		return nil
	}
	config := vmcpconfig.ConnectionConfiguration(component)
	credential, err := h.gatewayClient.RevealCredential(req.Ctx, []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return err
	}
	values := map[string]string{}
	for _, c := range config {
		if value, ok := credential.Secrets[vmcpconfig.ConfigurationKey(component.ID, c.Key)]; ok {
			values[c.Key] = value
		}
	}
	if err := h.gatewayClient.UpsertCredential(req.Ctx, gatewaytypes.Credential{
		Context: fmt.Sprintf("%s-%s", connection.Spec.UserID, connection.Name),
		Name:    connection.Name,
		Secrets: values,
	}); err != nil {
		return err
	}
	// Update the spec before the status: the client writes the new resource version back onto
	// the object, so the status update below still applies cleanly.
	_, legacyHash := connection.Annotations[v1.LegacyVMCPConnectionConfigurationHashAnnotation]
	if legacyHash || !vmcpconfig.SameConnectionConfiguration(connection.Spec.Config, config) {
		connection.Spec.Config = config
		delete(connection.Annotations, v1.LegacyVMCPConnectionConfigurationHashAnnotation)
		if err := req.Client.Update(req.Ctx, connection); err != nil {
			return err
		}
	}
	connection.Status.VMCPConfigurationHash = vmcpconfig.ConnectionConfigurationHash(config, instance)
	return req.Client.Status().Update(req.Ctx, connection)
}
