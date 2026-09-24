package oauth

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// Resolve configuration before building the aggregate or probing component OAuth.
// In particular, an instance may not have deployed its components yet.
func vmcpConsentTarget(req api.Context, connectID string) (*v1.VMCP, *v1.VMCPInstance, error) {
	vmcp, instance, err := vmcpconfig.ResolveConnectID(req.Context(), req.Storage, connectID, req.User.GetUID())
	if err != nil || vmcp == nil {
		return vmcp, instance, err
	}
	if instance == nil {
		instance, err = vmcpconfig.FindInstance(req.Context(), req.Storage, vmcp.Namespace, vmcp.Name, req.User.GetUID())
		if err != nil {
			return nil, nil, err
		}
	}
	if instance == nil {
		instance = &v1.VMCPInstance{
			Finalizers:   []string{v1.VMCPInstanceFinalizer},
			GenerateName: system.VMCPInstancePrefix,
			Namespace:    vmcp.Namespace,
			Spec: v1.VMCPInstanceSpec{
				Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				UserID:   req.User.GetUID(),
			},
		}
		if err := req.Create(instance); err != nil {
			return nil, nil, fmt.Errorf("create VMCP consent instance: %w", err)
		}
	}
	return vmcp, instance, nil
}

func vmcpConsentMissingConfiguration(req api.Context, vmcp v1.VMCP, instance v1.VMCPInstance) ([]string, error) {
	credential, err := req.GatewayClient.RevealCredential(req.Context(),
		[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
		vmcpconfig.ConfigurationCredentialName())
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, err
	}
	var missing []string
	for _, component := range vmcpConsentComponents(req.User, vmcp, instance) {
		missing = append(missing, vmcpconfig.MissingRequiredConfiguration(component, credential.Secrets, true)...)
	}
	return missing, nil
}

// vmcpConsentComponents omits components disabled for the user. The user never
// connects to them, so they require neither configuration nor OAuth.
func vmcpConsentComponents(u kuser.Info, vmcp v1.VMCP, instance v1.VMCPInstance) []types.VMCPComponent {
	return vmcpconfig.EnabledComponents(u, vmcp, vmcpconfig.ComponentsForInstance(vmcp, instance))
}

// vmcpConfigurationCheckHash matches the hash the VMCPInstance controller records
// once it has processed the given configuration. The controller resolves shared
// VMCP grants from the stored user, so do the same rather than trusting the request.
func vmcpConfigurationCheckHash(req api.Context, vmcp v1.VMCP, instance v1.VMCPInstance, syncHash string) (string, error) {
	u := req.User
	if vmcp.Spec.UserID == "" {
		id, err := strconv.ParseUint(instance.Spec.UserID, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid VMCP instance user ID: %w", err)
		}
		if u, err = req.GatewayClient.UserInfoByID(req.Context(), uint(id)); err != nil {
			return "", fmt.Errorf("resolve VMCP instance user: %w", err)
		}
	}
	return vmcpconfig.ConfigurationCheckHash(vmcpConsentComponents(u, vmcp, instance), syncHash), nil
}
