package handlers

import (
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

type VMCPHandler struct {
	acrHelper *accesscontrolrule.Helper
}

func NewVMCPHandler(acrHelper *accesscontrolrule.Helper) *VMCPHandler {
	return &VMCPHandler{acrHelper: acrHelper}
}

func (*VMCPHandler) List(req api.Context) error {
	var list v1.VMCPList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list VMCPs: %w", err)
	}

	all := (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true"

	items := make([]types.VMCP, 0, len(list.Items))
	for itemIndex := range list.Items {
		if all || authz.UserCanReadVMCP(req.User, &list.Items[itemIndex]) {
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

func (h *VMCPHandler) Create(req api.Context) error {
	var manifest types.VMCPManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP manifest: %v", err)
	}
	personalServer := !req.UserIsAdmin() || req.URL.Query().Get("scope") == "personal"

	manifest.Default(personalServer, req.User.GetUID())
	var userID string
	if personalServer {
		userID = req.User.GetUID()
	}

	if err := h.loadComponentSnapshots(req, &manifest, userID, nil); err != nil {
		return err
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}

	if err := vmcpconfig.InitializeComponentIDs(&manifest); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}

	staticConfiguration := vmcpconfig.ExtractStaticConfiguration(&manifest, nil)

	vmcp := v1.VMCP{
		Finalizers:   []string{v1.VMCPFinalizer},
		GenerateName: system.VMCPPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.VMCPSpec{
			Manifest:      manifest,
			UserID:        userID,
			CreatorUserID: req.User.GetUID(),
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
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(&vmcp, vmcp.Name); err != nil {
			return err
		}
		vmcpconfig.SetStaticConfigurationHashes(&vmcp, staticConfiguration)
		return req.Update(&vmcp)
	}); err != nil {
		cleanupErr := req.Delete(&vmcp)
		return errors.Join(fmt.Errorf("failed to publish VMCP static configuration: %w", err), cleanupErr)
	}
	return req.WriteCreated(convertVMCP(vmcp))
}

func (h *VMCPHandler) Update(req api.Context) error {
	var manifest types.VMCPManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP manifest: %v", err)
	}

	personalServer := !req.UserIsAdmin() || req.URL.Query().Get("scope") == "personal"
	manifest.Default(personalServer, req.User.GetUID())

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}
	if err := vmcpconfig.ReconcileComponentIDs(vmcp.Spec.Manifest, &manifest); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	vmcpconfig.PruneRemovedComponentProfiles(vmcp.Spec.Manifest.Components, &manifest)
	if err := h.loadComponentSnapshots(req, &manifest, vmcp.Spec.UserID, vmcp.Spec.Manifest.Components); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}

	credentialContext := vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)
	credentialName := vmcpconfig.ConfigurationCredentialName()

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{credentialContext}, credentialName)
	if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal VMCP static configuration: %w", err)
	}

	staticConfiguration := vmcpconfig.ExtractStaticConfiguration(&manifest, cred.Secrets)

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: credentialContext,
		Name:    credentialName,
		Secrets: staticConfiguration,
	}); err != nil {
		return fmt.Errorf("failed to store VMCP static configuration: %w", err)
	}
	vmcp.Spec.Manifest = manifest
	vmcpconfig.SetStaticConfigurationHashes(&vmcp, staticConfiguration)
	if err := req.Update(&vmcp); err != nil {
		return fmt.Errorf("failed to update VMCP: %w", err)
	}
	return req.Write(convertVMCP(vmcp))
}

// TriggerUpdate adopts current catalog snapshots in one resource update. It does
// not change policies or credentials; ordinary edits never refresh snapshots.
func (h *VMCPHandler) TriggerUpdate(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return err
	}
	if err := h.loadComponentSnapshots(req, &vmcp.Spec.Manifest, vmcp.Spec.UserID, nil); err != nil {
		return err
	}
	if err := vmcp.Spec.Manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP manifest: %v", err)
	}
	for _, component := range vmcp.Spec.Manifest.Components {
		if _, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true); err != nil {
			return types.NewErrBadRequest("invalid component %q: %v", component.Name, err)
		}
	}
	return req.Update(&vmcp)
}

func (*VMCPHandler) Reveal(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	credential, err := req.GatewayClient.RevealCredential(req.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil {
		if _, ok := errors.AsType[gclient.CredentialNotFoundError](err); !ok {
			return fmt.Errorf("failed to reveal VMCP configuration: %w", err)
		}
	}

	return req.Write(vmcpConfiguration(vmcp.Spec.Manifest.Components, credential.Secrets, types.VMCPConfigurationPolicyFixed))
}

func (*VMCPHandler) Deconfigure(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	if _, err := req.GatewayClient.DeleteCredential(req.Context(),
		vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		vmcpconfig.ConfigurationCredentialName(),
	); err != nil {
		return fmt.Errorf("failed to delete VMCP configuration: %w", err)
	}
	vmcpconfig.SetStaticConfigurationHashes(&vmcp, map[string]string{})
	if err := req.Update(&vmcp); err != nil {
		return fmt.Errorf("failed to update VMCP configuration hashes: %w", err)
	}
	return req.Write(convertVMCP(vmcp))
}

func (h *VMCPHandler) loadComponentSnapshots(req api.Context, manifest *types.VMCPManifest, ownerID string, existing []types.VMCPComponent) error {
	for i := range manifest.Components {
		component := &manifest.Components[i]
		if component.MCPServerCatalogEntryID == "" {
			return types.NewErrBadRequest("component mcpServerCatalogEntryID is required")
		}
		var previous *types.VMCPComponent
		for j := range existing {
			if existing[j].ID == component.ID && existing[j].MCPServerCatalogEntryID == component.MCPServerCatalogEntryID {
				previous = &existing[j]
				break
			}
		}
		var entry v1.MCPServerCatalogEntry
		err := req.Get(&entry, component.MCPServerCatalogEntryID)
		if err != nil && (previous == nil || !apierrors.IsNotFound(err)) {
			return fmt.Errorf("get component catalog entry %q: %w", component.MCPServerCatalogEntryID, err)
		}
		if err == nil {
			if err := authz.CheckVMCPComponentAccess(req.Context(), req.User, ownerID, &entry, h.acrHelper, req.GatewayClient.UserInfoByID); err != nil {
				return err
			}
		}
		if previous != nil {
			component.MCPCatalogID = previous.MCPCatalogID
			component.CatalogEntry = previous.CatalogEntry
			component.SourceDigest = previous.SourceDigest
			component.OAuthCredentialID = previous.OAuthCredentialID
			if component.Name == "" {
				component.Name = previous.Name
			}
			continue
		}
		component.MCPCatalogID = entry.Spec.MCPCatalogName
		component.CatalogEntry = types.MCPServerCatalogEntrySnapshot{
			Manifest:         entry.Spec.Manifest,
			UnsupportedTools: entry.Spec.UnsupportedTools,
		}
		component.SourceDigest = utils.Digest(component.CatalogEntry)
		component.OAuthCredentialID = vmcpconfig.StaticOAuthCredentialReference(entry.Spec.Manifest, entry.Name)
		if component.Name == "" {
			component.Name = entry.Spec.Manifest.Name
			if component.Name == "" {
				component.Name = entry.Name
			}
		}
	}
	return nil
}

func (*VMCPHandler) Delete(req api.Context) error {
	vmcpID := req.PathValue("vmcp_id")
	return req.Delete(&v1.VMCP{
		Name:      vmcpID,
		Namespace: req.Namespace(),
	})
}

func vmcpConfiguration(components []types.VMCPComponent, secrets map[string]string, policyType types.VMCPConfigurationPolicyType) types.VMCPConfiguration {
	configuration := types.VMCPConfiguration{Components: map[string]map[string]string{}}
	for _, component := range components {
		for _, policy := range component.Configuration {
			if policy.Policy != policyType {
				continue
			}
			value, ok := secrets[vmcpconfig.ConfigurationKey(component.ID, policy.Key)]
			if !ok {
				continue
			}
			if configuration.Components[component.ID] == nil {
				configuration.Components[component.ID] = make(map[string]string, 1)
			}
			configuration.Components[component.ID][policy.Key] = value
		}
	}
	return configuration
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
		LegacySlug:              vmcp.Spec.LegacySlug,
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
