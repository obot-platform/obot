package vmcpinstance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"

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
	"k8s.io/apimachinery/pkg/fields"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	revealCredential func(context.Context, []string, string) (gatewaytypes.Credential, error)
	userInfo         func(context.Context, uint) (kuser.Info, error)
}

func New(gatewayClient *gateway.Client) *Handler {
	handler := &Handler{}
	if gatewayClient != nil {
		handler.revealCredential = gatewayClient.RevealCredential
		handler.userInfo = gatewayClient.UserInfoByID
	}
	return handler
}

// ReconcileToolSelection permanently removes revoked tools from explicit selections.
func (h *Handler) ReconcileToolSelection(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)
	if len(instance.Spec.Manifest.EnabledTools) == 0 || !instance.DeletionTimestamp.IsZero() {
		return nil
	}
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); err != nil {
		return kclient.IgnoreNotFound(err)
	}
	if err := req.List(&v1.UserGroupChangeList{}, &kclient.ListOptions{Namespace: instance.Namespace}); err != nil {
		return err
	}
	id, err := strconv.ParseUint(instance.Spec.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid VMCP instance user ID: %w", err)
	}
	u, err := h.userInfo(req.Ctx, uint(id))
	if err != nil {
		return err
	}
	allowed := vmcpconfig.AllowedTools(u, vmcp.Spec.Manifest.Profiles, instance.Spec.Manifest.EnabledTools)
	allowed = slices.DeleteFunc(allowed, func(ref types.VMCPToolReference) bool {
		return vmcp.Spec.Manifest.ValidateToolReference(ref) != nil
	})
	if vmcp.Spec.UserID != "" && vmcp.Spec.UserID != instance.Spec.UserID {
		allowed = []types.VMCPToolReference{}
	}
	selection := types.ToolSetFromReferences(allowed)
	if reflect.DeepEqual(selection, instance.Spec.Manifest.EnabledTools) {
		return nil
	}
	instance.Spec.Manifest.EnabledTools = selection
	return req.Client.Update(req.Ctx, instance)
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
	effective := vmcp.Spec.Manifest
	effective.Components = vmcpconfig.ComponentsForInstance(vmcp, *instance)
	checkHash := utils.Digest([]any{effective.Components, instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation]})
	if instance.Status.ConfigurationCheckHash == checkHash {
		return nil
	}
	credential, err := h.revealCredential(req.Ctx,
		[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err == nil {
		configuration = userAllowedConfiguration(effective, credential.Secrets)
	} else if !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("reveal configuration credential for VMCP instance %q: %w", instance.Name, err)
	}

	var missing []string
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		missing = append(missing, vmcpconfig.MissingRequiredConfiguration(component, configuration, true)...)
	}
	slices.Sort(missing)
	configured := len(missing) == 0
	configurationHash := utils.Digest(configuration)
	instance.Status.ConfigurationCheckHash = checkHash
	instance.Status.UserConfigurationHash = configurationHash
	instance.Status.Configured = configured
	instance.Status.MissingRequiredConfiguration = missing
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
	if vmcpconfig.IsMultiUser(vmcp.Spec.Manifest) {
		var existing v1.MCPServerList
		if err := req.List(&existing, &kclient.ListOptions{Namespace: instance.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.vmcpInstanceID", instance.Name)}); err != nil {
			return err
		}
		for i := range existing.Items {
			if err := req.Client.Delete(req.Ctx, &existing.Items[i]); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}
		return nil
	}
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		server, err := mcpServerForComponent(instance, component)
		if err != nil {
			return fmt.Errorf("build MCPServer for VMCP component %q: %w", component.Name, err)
		}
		servers = append(servers, server)
	}
	desired := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		desired[server.Spec.VMCPComponentID] = struct{}{}
	}
	var existingServers v1.MCPServerList
	if err := req.List(&existingServers, &kclient.ListOptions{Namespace: instance.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.vmcpInstanceID", instance.Name)}); err != nil {
		return err
	}
	for i := range existingServers.Items {
		if _, ok := desired[existingServers.Items[i].Spec.VMCPComponentID]; ok {
			continue
		}
		if err := req.Client.Delete(req.Ctx, &existingServers.Items[i]); kclient.IgnoreNotFound(err) != nil {
			return err
		}
	}

	for index := range servers {
		server := &servers[index]
		var existing v1.MCPServer
		if err := req.Get(&existing, server.Namespace, server.Name); err == nil {
			if existing.Spec.VMCPInstanceID != instance.Name || existing.Spec.VMCPComponentID != server.Spec.VMCPComponentID {
				return fmt.Errorf("MCPServer %q already exists with different VMCP ownership", server.Name)
			}
			if existing.Annotations[v1.VMCPSnapshotDigestAnnotation] != server.Annotations[v1.VMCPSnapshotDigestAnnotation] {
				existing.Spec.Manifest = server.Spec.Manifest
				existing.Spec.UnsupportedTools = server.Spec.UnsupportedTools
				if existing.Annotations == nil {
					existing.Annotations = map[string]string{}
				}
				existing.Annotations[v1.VMCPSnapshotDigestAnnotation] = server.Annotations[v1.VMCPSnapshotDigestAnnotation]
				if err := req.Client.Update(req.Ctx, &existing); err != nil {
					return err
				}
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
		Name:        name.SafeConcatName(system.MCPServerPrefix+instance.Name, component.ID),
		Namespace:   instance.Namespace,
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: utils.Digest(component.CatalogEntry)},
		Spec: v1.MCPServerSpec{
			Manifest:         manifest,
			UnsupportedTools: slices.Clone(component.CatalogEntry.UnsupportedTools),
			UserID:           instance.Spec.UserID,
			VMCPInstanceID:   instance.Name,
			VMCPComponentID:  component.ID,
		},
	}, nil
}
