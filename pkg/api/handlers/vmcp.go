package handlers

import (
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
)

type VMCPHandler struct{}

func NewVMCPHandler() *VMCPHandler {
	return nil
}

func (*VMCPHandler) List(req api.Context) error {
	var list v1.VMCPList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list VMCPs: %w", err)
	}

	items := make([]types.VMCP, 0, len(list.Items))
	for itemIndex := range list.Items {
		if authz.UserCanReadVMCP(req.User, &list.Items[itemIndex]) {
			items = append(items, convertVMCP(list.Items[itemIndex]))
		}
	}
	return req.Write(types.VMCPList{Items: items})
}

func (*VMCPHandler) Get(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}
	return req.Write(convertVMCP(vmcp))
}

func (*VMCPHandler) Create(req api.Context) error {
	var manifest types.VMCPManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP manifest: %v", err)
	}
	manifest.Default()
	if err := authz.CheckVMCPForceSingleUser(req.User, false, manifest.ForceSingleUser); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	if err := vmcpconfig.InitializeComponentIDs(&manifest); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	staticConfiguration := vmcpconfig.ExtractStaticConfiguration(&manifest)
	var userID string
	if !req.UserIsAdmin() {
		userID = req.User.GetUID()
	}
	vmcp := v1.VMCP{
		Finalizers:   []string{v1.VMCPFinalizer},
		GenerateName: system.VMCPPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.VMCPSpec{
			Manifest:                manifest,
			UserID:                  userID,
			StaticConfigurationHash: utils.Digest(staticConfiguration),
		},
	}
	if err := req.Create(&vmcp); err != nil {
		return fmt.Errorf("failed to create VMCP: %w", err)
	}
	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: staticConfiguration,
	}); err != nil {
		cleanupErr := req.Delete(&vmcp)
		return errors.Join(fmt.Errorf("failed to store VMCP static configuration: %w", err), cleanupErr)
	}
	return req.WriteCreated(convertVMCP(vmcp))
}

func (*VMCPHandler) Update(req api.Context) error {
	var manifest types.VMCPManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP manifest: %v", err)
	}
	manifest.DefaultConfigurationPolicies()
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}
	if err := authz.CheckVMCPForceSingleUser(req.User, vmcp.Spec.Manifest.ForceSingleUser, manifest.ForceSingleUser); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	if err := vmcpconfig.ReconcileComponentIDs(vmcp.Spec.Manifest, &manifest); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	staticConfiguration := vmcpconfig.ExtractStaticConfiguration(&manifest)
	credentialContext := vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)
	credentialName := vmcpconfig.ConfigurationCredentialName()
	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: credentialContext,
		Name:    credentialName,
		Secrets: staticConfiguration,
	}); err != nil {
		return fmt.Errorf("failed to store VMCP static configuration: %w", err)
	}
	vmcp.Spec.Manifest = manifest
	vmcp.Spec.StaticConfigurationHash = utils.Digest(staticConfiguration)
	if err := req.Update(&vmcp); err != nil {
		return fmt.Errorf("failed to update VMCP: %w", err)
	}
	return req.Write(convertVMCP(vmcp))
}

func (*VMCPHandler) Delete(req api.Context) error {
	vmcpID := req.PathValue("vmcp_id")
	return req.Delete(&v1.VMCP{
		Name:      vmcpID,
		Namespace: req.Namespace(),
	})
}

func convertVMCP(vmcp v1.VMCP) types.VMCP {
	componentStatuses := make([]types.VMCPComponentStatus, 0, len(vmcp.Status.Components))
	for _, status := range vmcp.Status.Components {
		componentStatuses = append(componentStatuses, types.VMCPComponentStatus{
			Name:          status.Name,
			Ready:         status.Ready,
			Error:         status.Error,
			SourceMissing: status.SourceMissing,
			NeedsUpdate:   status.NeedsUpdate,
		})
	}
	return types.VMCP{
		Metadata:                MetadataFrom(&vmcp),
		VMCPManifest:            vmcp.Spec.Manifest,
		UserID:                  vmcp.Spec.UserID,
		StaticConfigurationHash: vmcp.Spec.StaticConfigurationHash,
		Status: types.VMCPStatus{
			Ready:      vmcp.Status.Ready,
			Components: componentStatuses,
		},
	}
}
