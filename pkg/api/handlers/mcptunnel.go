package handlers

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	tunnelpkg "github.com/obot-platform/obot/pkg/tunnel"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type mcpTunnelConnectionCloser interface {
	DisconnectCredential(name, credentialID string)
}

type MCPTunnelHandler struct {
	tunnels mcpTunnelConnectionCloser
}

func NewMCPTunnelHandler(tunnels mcpTunnelConnectionCloser) *MCPTunnelHandler {
	return &MCPTunnelHandler{tunnels: tunnels}
}

func (*MCPTunnelHandler) List(req api.Context) error {
	var list v1.MCPTunnelList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list MCP tunnels: %w", err)
	}

	items := make([]types.MCPTunnel, 0, len(list.Items))
	for _, item := range list.Items {
		converted, err := convertMCPTunnel(item, "")
		if err != nil {
			return fmt.Errorf("failed to convert MCP tunnel %q: %w", item.Name, err)
		}
		items = append(items, converted)
	}

	return req.Write(types.MCPTunnelList{Items: items})
}

func (*MCPTunnelHandler) Get(req api.Context) error {
	var tunnel v1.MCPTunnel
	if err := req.Get(&tunnel, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get MCP tunnel: %w", err)
	}

	converted, err := convertMCPTunnel(tunnel, "")
	if err != nil {
		return fmt.Errorf("failed to convert MCP tunnel %q: %w", tunnel.Name, err)
	}
	return req.Write(converted)
}

func (*MCPTunnelHandler) Create(req api.Context) error {
	manifest, err := readAndValidateMCPTunnelManifest(req)
	if err != nil {
		return err
	}

	token, credential, err := tunnelpkg.NewCredential()
	if err != nil {
		return err
	}
	credentialID, err := tunnelpkg.CredentialID(credential)
	if err != nil {
		return err
	}

	tunnel := v1.MCPTunnel{
		GenerateName: system.MCPTunnelPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.MCPTunnelSpec{
			Manifest:     manifest,
			Credential:   credential,
			CredentialID: credentialID,
		},
	}
	if err := req.Create(&tunnel); err != nil {
		return fmt.Errorf("failed to create MCP tunnel: %w", err)
	}

	converted, err := convertMCPTunnel(tunnel, token)
	if err != nil {
		return fmt.Errorf("failed to convert MCP tunnel %q: %w", tunnel.Name, err)
	}
	return req.WriteCreated(converted)
}

func (*MCPTunnelHandler) Update(req api.Context) error {
	manifest, err := readAndValidateMCPTunnelManifest(req)
	if err != nil {
		return err
	}

	var tunnel v1.MCPTunnel
	if err := req.Get(&tunnel, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get MCP tunnel: %w", err)
	}

	referencingEntries, err := listMCPTunnelCatalogEntries(req, tunnel.Name)
	if err != nil {
		return fmt.Errorf("failed to list MCP tunnel dependencies: %w", err)
	}
	disallowedReferences := disallowedMCPTunnelCatalogEntryReferences(referencingEntries, tunnel.Name, manifest)
	if len(disallowedReferences) > 0 {
		return types.NewErrBadRequest(
			"MCP tunnel %q cannot be updated because allowedURLs would no longer allow targets used by MCP server catalog entries: %s",
			mcpTunnelDisplayName(tunnel),
			strings.Join(disallowedReferences, ", "),
		)
	}

	tunnel.Spec.Manifest = manifest
	if err := req.Update(&tunnel); err != nil {
		return fmt.Errorf("failed to update MCP tunnel: %w", err)
	}

	converted, err := convertMCPTunnel(tunnel, "")
	if err != nil {
		return fmt.Errorf("failed to convert MCP tunnel %q: %w", tunnel.Name, err)
	}
	return req.Write(converted)
}

func (h *MCPTunnelHandler) Delete(req api.Context) error {
	var tunnel v1.MCPTunnel
	if err := req.Get(&tunnel, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get MCP tunnel: %w", err)
	}

	referencingEntries, err := listMCPTunnelCatalogEntries(req, tunnel.Name)
	if err != nil {
		return fmt.Errorf("failed to list MCP tunnel dependencies: %w", err)
	}
	references := formatMCPTunnelCatalogEntryReferences(referencingEntries)
	if len(references) > 0 {
		return types.NewErrBadRequest(
			"MCP tunnel %q cannot be deleted because it is used by MCP server catalog entries: %s",
			mcpTunnelDisplayName(tunnel),
			strings.Join(references, ", "),
		)
	}
	credentialID := tunnel.Spec.CredentialID
	if credentialID == "" {
		credentialID, _ = tunnelpkg.CredentialID(tunnel.Spec.Credential)
	}

	if err := req.Delete(&tunnel); err != nil {
		return fmt.Errorf("failed to delete MCP tunnel: %w", err)
	}
	if h.tunnels != nil {
		h.tunnels.DisconnectCredential(tunnel.Name, credentialID)
	}

	return req.Write(map[string]string{"deleted": tunnel.Name})
}

func mcpTunnelDisplayName(tunnel v1.MCPTunnel) string {
	if displayName := strings.TrimSpace(tunnel.Spec.Manifest.DisplayName); displayName != "" {
		return displayName
	}
	return tunnel.Name
}

func listMCPTunnelCatalogEntries(req api.Context, tunnelName string) ([]v1.MCPServerCatalogEntry, error) {
	var referencingEntries []v1.MCPServerCatalogEntry
	seen := map[kclient.ObjectKey]struct{}{}
	for _, fields := range []kclient.MatchingFields{
		{"spec.manifest.remoteConfig.tunnelName": tunnelName},
		{"spec.manifest.runtime": string(types.RuntimeComposite)},
	} {
		var list v1.MCPServerCatalogEntryList
		if err := req.List(&list, fields); err != nil {
			return nil, err
		}
		for _, entry := range list.Items {
			key := kclient.ObjectKeyFromObject(&entry)
			if _, ok := seen[key]; !ok && catalogEntryManifestUsesTunnel(entry.Spec.Manifest, tunnelName) {
				referencingEntries = append(referencingEntries, entry)
				seen[key] = struct{}{}
			}
		}
	}

	slices.SortFunc(referencingEntries, func(a, b v1.MCPServerCatalogEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	return referencingEntries, nil
}

func formatMCPTunnelCatalogEntryReferences(referencingEntries []v1.MCPServerCatalogEntry) []string {
	references := make([]string, 0, len(referencingEntries))
	for _, entry := range referencingEntries {
		references = append(references, formatMCPTunnelCatalogEntryReference(entry))
	}
	return references
}

func disallowedMCPTunnelCatalogEntryReferences(referencingEntries []v1.MCPServerCatalogEntry, tunnelName string, manifest types.MCPTunnelManifest) []string {
	var references []string
	for _, entry := range referencingEntries {
		for _, target := range catalogEntryManifestTunnelTargets(entry.Spec.Manifest, tunnelName) {
			if !manifest.AllowsURL(target) {
				references = append(references, fmt.Sprintf(
					"%s (target %q)",
					formatMCPTunnelCatalogEntryReference(entry),
					target,
				))
			}
		}
	}
	return references
}

func formatMCPTunnelCatalogEntryReference(entry v1.MCPServerCatalogEntry) string {
	displayName := strings.TrimSpace(entry.Spec.Manifest.Name)
	if displayName == "" || displayName == entry.Name {
		return entry.Name
	}
	return fmt.Sprintf("%q (%s)", displayName, entry.Name)
}

func catalogEntryManifestTunnelTargets(manifest types.MCPServerCatalogEntryManifest, tunnelName string) []string {
	switch manifest.Runtime {
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil || manifest.RemoteConfig.TunnelName != tunnelName {
			return nil
		}
		if manifest.RemoteConfig.FixedURL != "" {
			return []string{manifest.RemoteConfig.FixedURL}
		}
		if manifest.RemoteConfig.Hostname != "" {
			return []string{manifest.RemoteConfig.Hostname}
		}
	case types.RuntimeComposite:
		if manifest.CompositeConfig == nil {
			return nil
		}
		var targets []string
		for _, component := range manifest.CompositeConfig.ComponentServers {
			targets = append(targets, catalogEntryManifestTunnelTargets(component.Manifest, tunnelName)...)
		}
		return targets
	}
	return nil
}

func catalogEntryManifestUsesTunnel(manifest types.MCPServerCatalogEntryManifest, tunnelName string) bool {
	switch manifest.Runtime {
	case types.RuntimeRemote:
		return manifest.RemoteConfig != nil && manifest.RemoteConfig.TunnelName == tunnelName
	case types.RuntimeComposite:
		if manifest.CompositeConfig == nil {
			return false
		}
		for _, component := range manifest.CompositeConfig.ComponentServers {
			if catalogEntryManifestUsesTunnel(component.Manifest, tunnelName) {
				return true
			}
		}
	}
	return false
}

func (h *MCPTunnelHandler) RotateSecret(req api.Context) error {
	var tunnel v1.MCPTunnel
	if err := req.Get(&tunnel, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get MCP tunnel: %w", err)
	}

	previousCredentialID := tunnel.Spec.CredentialID
	if previousCredentialID == "" {
		// If this produces an error, then it's not possible to use the previous credential ID,
		// so we don't need to worry about disconnecting it.
		previousCredentialID, _ = tunnelpkg.CredentialID(tunnel.Spec.Credential)
	}
	token, credential, err := tunnelpkg.NewCredential()
	if err != nil {
		return err
	}
	credentialID, err := tunnelpkg.CredentialID(credential)
	if err != nil {
		return err
	}

	tunnel.Spec.Credential = credential
	tunnel.Spec.CredentialID = credentialID
	if err := req.Update(&tunnel); err != nil {
		return fmt.Errorf("failed to rotate MCP tunnel secret: %w", err)
	}
	if h.tunnels != nil {
		h.tunnels.DisconnectCredential(tunnel.Name, previousCredentialID)
	}

	converted, err := convertMCPTunnel(tunnel, token)
	if err != nil {
		return fmt.Errorf("failed to convert MCP tunnel %q: %w", tunnel.Name, err)
	}
	return req.Write(converted)
}

func readAndValidateMCPTunnelManifest(req api.Context) (types.MCPTunnelManifest, error) {
	var manifest types.MCPTunnelManifest
	if err := req.Read(&manifest); err != nil {
		return types.MCPTunnelManifest{}, types.NewErrBadRequest("failed to read MCP tunnel manifest: %v", err)
	}

	manifest.DisplayName = strings.TrimSpace(manifest.DisplayName)
	manifest.Description = strings.TrimSpace(manifest.Description)
	for i := range manifest.AllowedURLs {
		manifest.AllowedURLs[i] = strings.TrimSpace(manifest.AllowedURLs[i])
	}
	if err := manifest.Validate(); err != nil {
		return types.MCPTunnelManifest{}, types.NewErrBadRequest("invalid MCP tunnel: %v", err)
	}

	return manifest, nil
}

func convertMCPTunnel(tunnel v1.MCPTunnel, token string) (types.MCPTunnel, error) {
	if token == "" {
		var err error
		token, err = tunnelpkg.CredentialPreview(tunnel.Spec.Credential)
		if err != nil {
			return types.MCPTunnel{}, err
		}
	}

	return types.MCPTunnel{
		Metadata: MetadataFrom(&tunnel),
		Manifest: tunnel.Spec.Manifest,
		Token:    token,
	}, nil
}
