package mcpserverinstance

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
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
	config := slices.DeleteFunc(vmcpconfig.ComponentConfig(component), func(c types.MCPConfig) bool { return !c.UserAllowed })
	checkHash := utils.Digest([]any{config, instance.Status.UserConfigurationHash})
	const annotation = "obot.obot.ai/vmcp-instance-configuration-hash"
	if connection.Annotations[annotation] == checkHash && reflect.DeepEqual(connection.Spec.Config, config) {
		return nil
	}
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
	connection.Spec.Config = config
	if connection.Annotations == nil {
		connection.Annotations = map[string]string{}
	}
	connection.Annotations[annotation] = checkHash
	return req.Client.Update(req.Ctx, connection)
}
