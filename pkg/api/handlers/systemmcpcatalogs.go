package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	mcpcataloghandler "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SystemMCPCatalogHandler struct {
	defaultCatalogPath string
}

func NewSystemMCPCatalogHandler(defaultCatalogPath string) *SystemMCPCatalogHandler {
	return &SystemMCPCatalogHandler{defaultCatalogPath: defaultCatalogPath}
}

func (*SystemMCPCatalogHandler) List(req api.Context) error {
	var list v1.SystemMCPCatalogList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list system catalogs: %w", err)
	}

	items := make([]types.SystemMCPCatalog, 0, len(list.Items))
	for _, item := range list.Items {
		tokenEnv, err := revealCatalogTokens(req, item.Name)
		if err != nil {
			return err
		}
		items = append(items, convertSystemMCPCatalog(item, tokenEnv))
	}

	return req.Write(types.SystemMCPCatalogList{Items: items})
}

func (*SystemMCPCatalogHandler) Get(req api.Context) error {
	var catalog v1.SystemMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}
	tokenEnv, err := revealCatalogTokens(req, catalog.Name)
	if err != nil {
		return err
	}
	return req.Write(convertSystemMCPCatalog(catalog, tokenEnv))
}

func (h *SystemMCPCatalogHandler) Create(req api.Context) error {
	var manifest types.SystemMCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read system catalog manifest: %w", err)
	}
	if err := validateSystemCatalogManifest(&manifest, h.defaultCatalogPath); err != nil {
		return err
	}

	catalog := v1.SystemMCPCatalog{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SystemCatalogPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.SystemMCPCatalogSpec{
			DisplayName: manifest.DisplayName,
			SourceURLs:  manifest.SourceURLs,
		},
	}
	if err := req.Create(&catalog); err != nil {
		return fmt.Errorf("failed to create system catalog: %w", err)
	}
	newTokens := mergeCatalogTokens(manifest.SourceURLs, manifest.SourceURLCredentials, nil)
	if err := storeCatalogTokens(req, catalog.Name, newTokens, nil); err != nil {
		return err
	}

	return req.Write(convertSystemMCPCatalog(catalog, newTokens))
}

func (h *SystemMCPCatalogHandler) Update(req api.Context) error {
	var manifest types.SystemMCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read system catalog manifest: %w", err)
	}
	if err := validateSystemCatalogManifest(&manifest, h.defaultCatalogPath); err != nil {
		return err
	}

	var catalog v1.SystemMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}

	existingCred, err := req.GPTClient.RevealCredential(req.Context(), []string{catalog.Name}, mcpcataloghandler.CatalogCredentialToolName)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal system catalog credentials: %w", err)
	}

	newTokens := mergeCatalogTokens(manifest.SourceURLs, manifest.SourceURLCredentials, existingCred.Env)
	catalog.Spec.DisplayName = manifest.DisplayName
	catalog.Spec.SourceURLs = manifest.SourceURLs
	if err := req.Update(&catalog); err != nil {
		return fmt.Errorf("failed to update system catalog: %w", err)
	}
	if err := storeCatalogTokens(req, catalog.Name, newTokens, existingCred.Env); err != nil {
		return err
	}

	return req.Write(convertSystemMCPCatalog(catalog, newTokens))
}

func (*SystemMCPCatalogHandler) Delete(req api.Context) error {
	var catalog v1.SystemMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}
	if err := req.Delete(&catalog); err != nil {
		return fmt.Errorf("failed to delete system catalog: %w", err)
	}
	return req.Write(map[string]string{"deleted": catalog.Name})
}

func (*SystemMCPCatalogHandler) Refresh(req api.Context) error {
	var catalog v1.SystemMCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}
	if catalog.Annotations == nil {
		catalog.Annotations = make(map[string]string)
	}
	catalog.Annotations[v1.SystemMCPCatalogSyncAnnotation] = "true"
	return req.Update(&catalog)
}

func (*SystemMCPCatalogHandler) ListEntries(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	if err := req.Get(&v1.SystemMCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}

	var list v1.SystemMCPServerCatalogEntryList
	if err := req.List(&list, client.MatchingFields{"spec.systemMCPCatalogName": catalogName}); err != nil {
		return fmt.Errorf("failed to list system catalog entries: %w", err)
	}

	entries := make([]types.SystemMCPServerCatalogEntry, 0, len(list.Items))
	for _, entry := range list.Items {
		entries = append(entries, ConvertSystemMCPServerCatalogEntry(entry))
	}
	return req.Write(types.SystemMCPServerCatalogEntryList{Items: entries})
}

func (*SystemMCPCatalogHandler) GetEntry(req api.Context) error {
	entry, err := getSystemCatalogEntry(req)
	if err != nil {
		return err
	}
	return req.Write(ConvertSystemMCPServerCatalogEntry(*entry))
}

func (*SystemMCPCatalogHandler) CreateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	if err := req.Get(&v1.SystemMCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get system catalog: %w", err)
	}

	var manifest types.SystemMCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}
	if err := validation.ValidateSystemMCPServerCatalogEntryManifest(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	entry := v1.SystemMCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeHashConcatName(catalogName, normalizeMCPCatalogEntryName(manifest.Name)),
			Namespace:    req.Namespace(),
		},
		Spec: v1.SystemMCPServerCatalogEntrySpec{
			SystemMCPCatalogName: catalogName,
			Editable:             true,
			Manifest:             manifest,
		},
	}
	if err := req.Create(&entry); err != nil {
		return fmt.Errorf("failed to create system catalog entry: %w", err)
	}
	return req.Write(ConvertSystemMCPServerCatalogEntry(entry))
}

func (*SystemMCPCatalogHandler) UpdateEntry(req api.Context) error {
	entry, err := getSystemCatalogEntry(req)
	if err != nil {
		return err
	}
	if !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	var manifest types.SystemMCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}
	if err := validation.ValidateSystemMCPServerCatalogEntryManifest(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
	entry.Spec.Manifest = manifest
	if err := req.Update(entry); err != nil {
		return fmt.Errorf("failed to update system catalog entry: %w", err)
	}
	return req.Write(ConvertSystemMCPServerCatalogEntry(*entry))
}

func (*SystemMCPCatalogHandler) DeleteEntry(req api.Context) error {
	entry, err := getSystemCatalogEntry(req)
	if err != nil {
		return err
	}
	if !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable and cannot be manually deleted")
	}
	if err := req.Delete(entry); err != nil {
		return fmt.Errorf("failed to delete system catalog entry: %w", err)
	}
	return req.Write(map[string]string{"deleted": entry.Name})
}

func getSystemCatalogEntry(req api.Context) (*v1.SystemMCPServerCatalogEntry, error) {
	catalogName := req.PathValue("catalog_id")
	if err := req.Get(&v1.SystemMCPCatalog{}, catalogName); err != nil {
		return nil, fmt.Errorf("failed to get system catalog: %w", err)
	}

	var entry v1.SystemMCPServerCatalogEntry
	if err := req.Get(&entry, req.PathValue("entry_id")); err != nil {
		return nil, fmt.Errorf("failed to get system catalog entry: %w", err)
	}
	if entry.Spec.SystemMCPCatalogName != catalogName {
		return nil, types.NewErrBadRequest("entry does not belong to system catalog")
	}
	return &entry, nil
}

func validateSystemCatalogManifest(manifest *types.SystemMCPCatalogManifest, localPath string) error {
	return normalizeAndValidateCatalogSourceURLs(manifest.SourceURLs, localPath)
}

func normalizeAndValidateCatalogSourceURLs(sourceURLs []string, localPath string) error {
	for i, urlStr := range sourceURLs {
		if urlStr == "" || urlStr == localPath {
			continue
		}
		if !strings.Contains(urlStr, "://") {
			urlStr = "https://" + urlStr
			sourceURLs[i] = urlStr
		}
		u, err := url.Parse(urlStr)
		if err != nil {
			return types.NewErrBadRequest("invalid URL: %v", err)
		}
		if u.Scheme != "https" {
			return types.NewErrBadRequest("only HTTPS URLs are supported")
		}
	}

	seen := make(map[string]struct{}, len(sourceURLs))
	for _, urlStr := range sourceURLs {
		if urlStr == "" {
			continue
		}
		if _, ok := seen[urlStr]; ok {
			return types.NewErrBadRequest("duplicate URL found: %s", urlStr)
		}
		seen[urlStr] = struct{}{}
	}
	return nil
}

func mergeCatalogTokens(sourceURLs []string, incoming, existing map[string]string) map[string]string {
	activeURLs := make(map[string]struct{}, len(sourceURLs))
	for _, u := range sourceURLs {
		activeURLs[u] = struct{}{}
	}

	newTokens := make(map[string]string)
	for u, token := range incoming {
		if _, active := activeURLs[u]; !active {
			continue
		}
		switch token {
		case "":
		case "*":
			if existingToken, ok := existing[u]; ok {
				newTokens[u] = existingToken
			}
		default:
			newTokens[u] = token
		}
	}
	for _, u := range sourceURLs {
		if _, mentioned := incoming[u]; mentioned {
			continue
		}
		if existingToken, ok := existing[u]; ok {
			newTokens[u] = existingToken
		}
	}
	return newTokens
}

func storeCatalogTokens(req api.Context, catalogName string, tokens, existing map[string]string) error {
	if len(tokens) > 0 {
		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  catalogName,
			ToolName: mcpcataloghandler.CatalogCredentialToolName,
			Type:     gptscript.CredentialTypeTool,
			Env:      tokens,
		}); err != nil {
			return fmt.Errorf("failed to store catalog credentials: %w", err)
		}
	} else if len(existing) > 0 {
		if err := req.GPTClient.DeleteCredential(req.Context(), catalogName, mcpcataloghandler.CatalogCredentialToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to delete catalog credentials: %w", err)
		}
	}
	return nil
}

func maskCatalogCredentials(sourceURLs []string, tokenEnv map[string]string) map[string]string {
	var maskedCredentials map[string]string
	for _, u := range sourceURLs {
		if _, ok := tokenEnv[u]; ok {
			if maskedCredentials == nil {
				maskedCredentials = make(map[string]string)
			}
			maskedCredentials[u] = "*"
		}
	}
	return maskedCredentials
}

func convertSystemMCPCatalog(catalog v1.SystemMCPCatalog, tokenEnv map[string]string) types.SystemMCPCatalog {
	return types.SystemMCPCatalog{
		Metadata: MetadataFrom(&catalog),
		SystemMCPCatalogManifest: types.SystemMCPCatalogManifest{
			DisplayName:          catalog.Spec.DisplayName,
			SourceURLs:           catalog.Spec.SourceURLs,
			SourceURLCredentials: maskCatalogCredentials(catalog.Spec.SourceURLs, tokenEnv),
		},
		LastSynced: *types.NewTime(catalog.Status.LastSyncTime.Time),
		SyncErrors: catalog.Status.SyncErrors,
		IsSyncing:  catalog.Status.IsSyncing || catalog.Annotations[v1.SystemMCPCatalogSyncAnnotation] == "true",
	}
}

func ConvertSystemMCPServerCatalogEntry(entry v1.SystemMCPServerCatalogEntry) types.SystemMCPServerCatalogEntry {
	return types.SystemMCPServerCatalogEntry{
		Metadata:                  MetadataFrom(&entry),
		Manifest:                  entry.Spec.Manifest,
		Editable:                  entry.Spec.Editable,
		CatalogName:               entry.Spec.SystemMCPCatalogName,
		SourceURL:                 entry.Spec.SourceURL,
		LastUpdated:               v1.NewTime(entry.Status.LastUpdated),
		ToolPreviewsLastGenerated: v1.NewTime(entry.Status.ToolPreviewsLastGenerated),
		NeedsUpdate:               entry.Status.NeedsUpdate,
		OAuthCredentialConfigured: entry.Status.OAuthCredentialConfigured,
	}
}
