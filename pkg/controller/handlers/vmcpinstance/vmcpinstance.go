package vmcpinstance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// DeleteUnauthorized removes shared VMCP instances whose user no longer matches any profile.
func (h *Handler) DeleteUnauthorized(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); err != nil {
		return kclient.IgnoreNotFound(err)
	}

	if vmcp.Spec.UserID != "" {
		// If the vMCP is a personal vMCP, then it will be deleted when it should no longer exist.
		// Then this instance will be cleaned up on vMCP deletion by reference.
		return nil
	}

	// Register a trigger so group membership changes also recheck profile access.
	if err := req.List(&v1.UserGroupChangeList{}, &kclient.ListOptions{
		Namespace:     instance.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", instance.Spec.UserID),
	}); err != nil {
		return err
	}

	if err := req.List(&v1.UserRoleChangeList{}, &kclient.ListOptions{
		Namespace:     instance.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", instance.Spec.UserID),
	}); err != nil {
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

	if len(vmcpconfig.MatchingProfiles(u, vmcp.Spec.Manifest.Profiles)) > 0 {
		return nil
	}

	slog.Info("Deleting VMCPInstance after profile access loss", "instance", instance.Name, "userID", instance.Spec.UserID)
	return kclient.IgnoreNotFound(req.Delete(instance))
}

// ReconcileToolSelection permanently removes revoked tools from explicit selections.
func (h *Handler) ReconcileToolSelection(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)
	if len(instance.Spec.Manifest.ComponentSet) == 0 {
		return nil
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); err != nil {
		return kclient.IgnoreNotFound(err)
	}

	allowed := []types.VMCPToolReference{}
	switch vmcp.Spec.UserID {
	case "":
		// Register a trigger on group list changes so we recalculate when things change.
		if err := req.List(&v1.UserGroupChangeList{}, &kclient.ListOptions{
			Namespace:     instance.Namespace,
			FieldSelector: fields.OneTermEqualSelector("spec.userID", instance.Spec.UserID),
		}); err != nil {
			return err
		}

		// Same for user role changes.
		if err := req.List(&v1.UserRoleChangeList{}, &kclient.ListOptions{
			Namespace:     instance.Namespace,
			FieldSelector: fields.OneTermEqualSelector("spec.userID", instance.Spec.UserID),
		}); err != nil {
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

		allowed = vmcpconfig.AllowedTools(u, vmcp.Spec.Manifest.Profiles, instance.Spec.Manifest.ComponentSet)
	case instance.Spec.UserID:
		allowed = types.ComponentToolReferences(instance.Spec.Manifest.ComponentSet)
	}

	allowed = slices.DeleteFunc(allowed, func(ref types.VMCPToolReference) bool {
		return vmcp.Spec.Manifest.ValidateToolReference(ref) != nil
	})

	selection := types.ComponentsFromToolReferences(allowed)

	if reflect.DeepEqual(selection, instance.Spec.Manifest.ComponentSet) {
		return nil
	}

	instance.Spec.Manifest.ComponentSet = selection
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
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		if vmcpconfig.IsMultiUser(component) {
			continue
		}
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

			if existing.Annotations[v1.VMCPSnapshotDigestAnnotation] != server.Annotations[v1.VMCPSnapshotDigestAnnotation] ||
				existing.Spec.UserID != server.Spec.UserID ||
				existing.Spec.MCPServerCatalogEntryName != server.Spec.MCPServerCatalogEntryName ||
				!reflect.DeepEqual(existing.Spec.Manifest.Config, server.Spec.Manifest.Config) {
				existing.Spec.Manifest = server.Spec.Manifest
				existing.Spec.UnsupportedTools = server.Spec.UnsupportedTools
				existing.Spec.UserID = server.Spec.UserID
				existing.Spec.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName
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

	// Dedicated servers consume both fixed and user configuration from their
	// synchronized credential, never from shared-server passthrough headers.
	catalogManifest := component.CatalogEntry.Manifest.DeepCopy()
	for i := range catalogManifest.Config {
		catalogManifest.Config[i].UserAllowed = false
	}
	manifest, err := types.MapCatalogEntryToServer(*catalogManifest, "", true)
	if err != nil {
		return v1.MCPServer{}, err
	}

	return v1.MCPServer{
		Name:        name.SafeConcatName(system.MCPServerPrefix+instance.Name, component.ID),
		Namespace:   instance.Namespace,
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: utils.Digest(component.CatalogEntry)},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: component.MCPServerCatalogEntryID,
			Manifest:                  manifest,
			UnsupportedTools:          slices.Clone(component.CatalogEntry.UnsupportedTools),
			UserID:                    instance.Spec.UserID,
			VMCPInstanceID:            instance.Name,
			VMCPComponentID:           component.ID,
		},
	}, nil
}
