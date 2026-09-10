package handlers

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	mcpcataloghandler "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type VMCPCatalogHandler struct {
	defaultCatalogPaths string
}

func NewVMCPCatalogHandler(defaultCatalogPaths string) *VMCPCatalogHandler {
	return &VMCPCatalogHandler{defaultCatalogPaths: defaultCatalogPaths}
}

func (*VMCPCatalogHandler) List(req api.Context) error {
	var list v1.VMCPCatalogList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list VMCP catalogs: %w", err)
	}

	items := make([]types.VMCPCatalog, 0, len(list.Items))
	for _, item := range list.Items {
		tokens, err := revealCatalogTokens(req, mcpcataloghandler.VMCPCatalogCredentialContext(item.Name))
		if err != nil {
			return err
		}
		items = append(items, convertVMCPCatalog(item, tokens))
	}
	return req.Write(types.VMCPCatalogList{Items: items})
}

func (*VMCPCatalogHandler) Get(req api.Context) error {
	var catalog v1.VMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get VMCP catalog: %w", err)
	}
	tokens, err := revealCatalogTokens(req, mcpcataloghandler.VMCPCatalogCredentialContext(catalog.Name))
	if err != nil {
		return err
	}
	return req.Write(convertVMCPCatalog(catalog, tokens))
}

func (h *VMCPCatalogHandler) Create(req api.Context) error {
	var manifest types.VMCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read VMCP catalog manifest: %w", err)
	}
	originalSourceURLs := slices.Clone(manifest.SourceURLs)
	if err := validateVMCPCatalogManifest(&manifest, h.defaultCatalogPaths); err != nil {
		return err
	}
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLCredentials)
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs)
	if err := validateCatalogGitCredentials(req, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs); err != nil {
		return err
	}

	catalog := v1.VMCPCatalog{
		GenerateName: system.VMCPCatalogPrefix,
		Namespace:    req.Namespace(),
		Finalizers:   []string{v1.VMCPCatalogFinalizer},
		Spec: v1.VMCPCatalogSpec{
			DisplayName:               manifest.DisplayName,
			SourceURLs:                manifest.SourceURLs,
			SourceURLGitCredentialIDs: manifest.SourceURLGitCredentialIDs,
		},
	}
	if err := req.Create(&catalog); err != nil {
		return fmt.Errorf("failed to create VMCP catalog: %w", err)
	}
	tokens := mergeCatalogTokens(manifest.SourceURLs, manifest.SourceURLCredentials, nil)
	removeSharedCredentialTokens(tokens, manifest.SourceURLGitCredentialIDs)
	if err := storeCatalogTokens(req, mcpcataloghandler.VMCPCatalogCredentialContext(catalog.Name), tokens, nil); err != nil {
		return err
	}
	return req.Write(convertVMCPCatalog(catalog, tokens))
}

func (h *VMCPCatalogHandler) Update(req api.Context) error {
	var manifest types.VMCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read VMCP catalog manifest: %w", err)
	}
	originalSourceURLs := slices.Clone(manifest.SourceURLs)
	if err := validateVMCPCatalogManifest(&manifest, h.defaultCatalogPaths); err != nil {
		return err
	}
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLCredentials)
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs)
	if err := validateCatalogGitCredentials(req, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs); err != nil {
		return err
	}

	var catalog v1.VMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get VMCP catalog: %w", err)
	}
	existingCred, err := req.GatewayClient.RevealCredential(req.Context(), []string{mcpcataloghandler.VMCPCatalogCredentialContext(catalog.Name)}, mcpcataloghandler.CatalogCredentialToolName)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal VMCP catalog credentials: %w", err)
	}
	tokens := mergeCatalogTokens(manifest.SourceURLs, manifest.SourceURLCredentials, existingCred.Secrets)
	removeSharedCredentialTokens(tokens, manifest.SourceURLGitCredentialIDs)
	catalog.Spec.DisplayName = manifest.DisplayName
	catalog.Spec.SourceURLs = manifest.SourceURLs
	catalog.Spec.SourceURLGitCredentialIDs = manifest.SourceURLGitCredentialIDs
	if err := req.Update(&catalog); err != nil {
		return fmt.Errorf("failed to update VMCP catalog: %w", err)
	}
	if err := storeCatalogTokens(req, mcpcataloghandler.VMCPCatalogCredentialContext(catalog.Name), tokens, existingCred.Secrets); err != nil {
		return err
	}
	return req.Write(convertVMCPCatalog(catalog, tokens))
}

func (*VMCPCatalogHandler) Delete(req api.Context) error {
	var catalog v1.VMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get VMCP catalog: %w", err)
	}
	if err := req.Delete(&catalog); err != nil {
		return fmt.Errorf("failed to delete VMCP catalog: %w", err)
	}
	return req.Write(map[string]string{"deleted": catalog.Name})
}

func (*VMCPCatalogHandler) Refresh(req api.Context) error {
	var catalog v1.VMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get VMCP catalog: %w", err)
	}
	if catalog.Annotations == nil {
		catalog.Annotations = make(map[string]string)
	}
	catalog.Annotations[v1.VMCPCatalogSyncAnnotation] = "true"
	return req.Update(&catalog)
}

func validateVMCPCatalogManifest(manifest *types.VMCPCatalogManifest, configuredPaths string) error {
	var paths []string
	for path := range strings.SplitSeq(configuredPaths, ",") {
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	return normalizeAndValidateCatalogSourceURLs(manifest.SourceURLs, paths)
}

func convertVMCPCatalog(catalog v1.VMCPCatalog, tokens map[string]string) types.VMCPCatalog {
	return types.VMCPCatalog{
		Metadata:                  MetadataFrom(&catalog),
		DisplayName:               catalog.Spec.DisplayName,
		SourceURLs:                catalog.Spec.SourceURLs,
		SourceURLCredentials:      maskCatalogCredentials(catalog.Spec.SourceURLs, tokens),
		SourceURLGitCredentialIDs: catalog.Spec.SourceURLGitCredentialIDs,
		LastSynced:                *types.NewTime(catalog.Status.LastSyncTime.Time),
		SyncErrors:                catalog.Status.SyncErrors,
		IsSyncing:                 catalog.Status.IsSyncing || catalog.Annotations[v1.VMCPCatalogSyncAnnotation] == "true",
	}
}
