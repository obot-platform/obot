package handlers

import (
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type VMCPInstanceHandler struct{}

func NewVMCPInstanceHandler() *VMCPInstanceHandler {
	return nil
}

func (*VMCPInstanceHandler) List(req api.Context) error {
	var (
		list   v1.VMCPInstanceList
		fields = kclient.MatchingFields{}
	)
	if !req.UserIsAdmin() {
		fields["spec.userID"] = req.User.GetUID()
	}
	if err := req.List(&list, fields); err != nil {
		return fmt.Errorf("failed to list VMCP instances: %w", err)
	}

	items := make([]types.VMCPInstance, 0, len(list.Items))
	for _, item := range list.Items {
		if !req.UserIsAdmin() {
			var vmcp v1.VMCP
			if err := req.Get(&vmcp, item.Spec.Manifest.VMCPID); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("failed to get VMCP for instance %q: %w", item.Name, err)
			}
			if !authz.UserCanReadVMCPInstance(req.User, &item, &vmcp) {
				continue
			}
		}
		items = append(items, convertVMCPInstance(item))
	}
	return req.Write(types.VMCPInstanceList{Items: items})
}

func (*VMCPInstanceHandler) Get(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	return req.Write(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Create(req api.Context) error {
	var manifest types.VMCPInstanceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP instance manifest: %v", err)
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, manifest.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	if !authz.UserCanReadVMCP(req.User, &vmcp) {
		return types.NewErrForbidden("access denied to VMCP %q", manifest.VMCPID)
	}

	if err := authz.ValidateVMCPToolSelection(req.User, &vmcp, manifest.EnabledTools); err != nil {
		return err
	}

	var existing v1.VMCPInstanceList
	if err := req.List(&existing, kclient.MatchingFields{
		"spec.userID":          req.User.GetUID(),
		"spec.manifest.vmcpID": manifest.VMCPID,
	}); err != nil {
		return fmt.Errorf("failed to find existing VMCP instance: %w", err)
	}
	if len(existing.Items) > 1 {
		return fmt.Errorf("found multiple VMCP instances for VMCP %q and user %q", manifest.VMCPID, req.User.GetUID())
	}
	if len(existing.Items) == 1 {
		return req.WriteCreated(convertVMCPInstance(existing.Items[0]))
	}

	instance := v1.VMCPInstance{
		Finalizers:   []string{v1.VMCPInstanceFinalizer},
		GenerateName: system.VMCPInstancePrefix,
		Namespace:    req.Namespace(),
		Spec: v1.VMCPInstanceSpec{
			Manifest: manifest,
			UserID:   req.User.GetUID(),
		},
	}
	if err := req.Create(&instance); err != nil {
		return fmt.Errorf("failed to create VMCP instance: %w", err)
	}
	return req.WriteCreated(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Update(req api.Context) error {
	var manifest types.VMCPInstanceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP instance manifest: %v", err)
	}

	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	if manifest.VMCPID != instance.Spec.Manifest.VMCPID {
		return types.NewErrBadRequest("vmcpID cannot be changed")
	}
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, manifest.VMCPID); err != nil {
		return err
	}
	selectionUser := req.User
	if instance.Spec.UserID != req.User.GetUID() && len(manifest.EnabledTools) > 0 {
		id, err := strconv.ParseUint(instance.Spec.UserID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid VMCP instance user ID: %w", err)
		}
		owner, err := req.GatewayClient.UserInfoByID(req.Context(), uint(id))
		if err != nil {
			return err
		}
		selectionUser = owner
	}
	if err := authz.ValidateVMCPToolSelection(selectionUser, &vmcp, manifest.EnabledTools); err != nil {
		return err
	}
	instance.Spec.Manifest = manifest
	if err := req.Update(&instance); err != nil {
		return fmt.Errorf("failed to update VMCP instance: %w", err)
	}
	return req.Write(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Delete(req api.Context) error {
	instanceID := req.PathValue("vmcp_instance_id")
	return req.Delete(&v1.VMCPInstance{
		Name:      instanceID,
		Namespace: req.Namespace(),
	})
}

func (*VMCPInstanceHandler) Configure(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Spec.Manifest.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	var configuration types.VMCPConfiguration
	if err := req.Read(&configuration); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance configuration: %v", err)
	}
	secrets, err := vmcpconfig.ValidateAndEncodeUserConfiguration(vmcp.Spec.Manifest, configuration)
	if err != nil {
		return types.NewErrBadRequest("invalid VMCP instance configuration: %v", err)
	}
	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: secrets,
	}); err != nil {
		return fmt.Errorf("failed to store VMCP instance configuration: %w", err)
	}
	if instance.Annotations == nil {
		instance.Annotations = map[string]string{}
	}
	instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation] = utils.Digest(secrets)
	if err := req.Update(&instance); err != nil {
		return fmt.Errorf("failed to trigger VMCP instance configuration reconciliation: %w", err)
	}
	return req.Write(convertVMCPInstance(instance))
}

func convertVMCPInstance(instance v1.VMCPInstance) types.VMCPInstance {
	return types.VMCPInstance{
		Metadata:             MetadataFrom(&instance),
		VMCPInstanceManifest: instance.Spec.Manifest,
		UserID:               instance.Spec.UserID,
		Status: types.VMCPInstanceStatus{
			Configured:                   instance.Status.Configured,
			MissingRequiredConfiguration: instance.Status.MissingRequiredConfiguration,
			UserConfigurationHash:        instance.Status.UserConfigurationHash,
		},
	}
}
