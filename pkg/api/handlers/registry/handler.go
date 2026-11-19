package registry

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	serverURL string
}

func NewHandler(serverURL string) *Handler {
	return &Handler{serverURL: serverURL}
}

// ListServers handles GET /v0.1/servers.
func (h *Handler) ListServers(req api.Context) error {
	reverseDNS, err := ReverseDNSFromURL(h.serverURL)
	if err != nil {
		return fmt.Errorf("failed to generate reverse DNS: %w", err)
	}

	var list v1.VMCPList
	if err := req.Storage.List(req.Context(), &list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return fmt.Errorf("failed to list vMCPs: %w", err)
	}

	definitions := make([]v1.VMCP, 0, len(list.Items))
	search := strings.ToLower(req.URL.Query().Get("search"))
	for _, item := range list.Items {
		if !registryVMCPVisible(req, &item) {
			continue
		}
		// Search the full manifest description before conversion truncates it.
		if search != "" &&
			!strings.Contains(strings.ToLower(FormatRegistryServerName(reverseDNS, item.Name)), search) &&
			!strings.Contains(strings.ToLower(item.Spec.Manifest.DisplayName), search) &&
			!strings.Contains(strings.ToLower(item.Spec.Manifest.Description), search) {
			continue
		}
		definitions = append(definitions, item)
	}
	return req.Write(h.paginateVMCPs(definitions, reverseDNS, req.URL.Query().Get("cursor"), parseLimit(req.URL.Query().Get("limit"))))
}

// Registry visibility follows consumption permissions, including for administrators.
// Anonymous discovery deliberately publishes only shared definitions with an explicit
// wildcard profile, without interpreting anonymous user IDs or groups as grants.
func registryVMCPVisible(req api.Context, definition *v1.VMCP) bool {
	// Empty drafts cannot accept connections through the advertised remote.
	if len(definition.Spec.Manifest.Components) == 0 {
		return false
	}
	if req.User != nil && req.UserIsAuthenticated() {
		return authz.UserCanReadVMCP(req.User, definition)
	}
	if definition.IsPersonal() {
		return false
	}
	for _, profile := range definition.Spec.Manifest.Profiles {
		for _, subject := range profile.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				return true
			}
		}
	}
	return false
}

// Pagination and filtering helpers

func parseLimit(limitStr string) int {
	if limitStr == "" {
		return 50 // Default limit
	}

	var limit int
	_, _ = fmt.Sscanf(limitStr, "%d", &limit)

	if limit <= 0 || limit > 100 {
		return 50
	}

	return limit
}

func (h *Handler) paginateVMCPs(definitions []v1.VMCP, reverseDNS, cursor string, limit int) types.RegistryServerList {
	slices.SortStableFunc(definitions, func(i, j v1.VMCP) int {
		if result := cmp.Compare(i.CreationTimestamp.Unix(), j.CreationTimestamp.Unix()); result != 0 {
			return result
		}
		return strings.Compare(i.Name, j.Name)
	})

	start := 0
	if cursor != "" {
		for i, definition := range definitions {
			if FormatRegistryServerName(reverseDNS, definition.Name) == cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(definitions))
	metadata := &types.RegistryServerListMetadata{Count: end - start}
	if end < len(definitions) {
		metadata.NextCursor = FormatRegistryServerName(reverseDNS, definitions[end-1].Name)
	}
	servers := make([]types.RegistryServerResponse, 0, end-start)
	for _, definition := range definitions[start:end] {
		servers = append(servers, ConvertVMCPToRegistry(definition, h.serverURL, reverseDNS))
	}
	return types.RegistryServerList{Servers: servers, Metadata: metadata}
}

// ListServerVersions handles GET /v0.1/servers/{serverName}/versions
func (h *Handler) ListServerVersions(req api.Context) error {
	// Extract server name from path (should be in format: reverseDNS/serverName)
	serverName := req.PathValue("serverName")
	if serverName == "" {
		return fmt.Errorf("serverName is required")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server
	server, err := h.findServerByName(req, actualServerName, reverseDNS)
	if err != nil {
		return err
	}

	// Return as a ServerList with single item (only "latest" version exists)
	response := types.RegistryServerList{
		Servers: []types.RegistryServerResponse{server},
		Metadata: &types.RegistryServerListMetadata{
			Count: 1,
		},
	}

	return req.Write(response)
}

// GetServerVersion handles GET /v0.1/servers/{serverName}/versions/{version}
func (h *Handler) GetServerVersion(req api.Context) error {
	// Extract parameters from path (serverName should be in format: reverseDNS/serverName)
	serverName := req.PathValue("serverName")
	version := req.PathValue("version")

	if serverName == "" {
		return fmt.Errorf("serverName is required")
	}
	if version == "" {
		return fmt.Errorf("version is required")
	}

	// Only support "latest" version
	if version != "latest" {
		return h.notFoundError("Version not found. Only 'latest' is supported.")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server
	server, err := h.findServerByName(req, actualServerName, reverseDNS)
	if err != nil {
		return err
	}

	return req.Write(server)
}

// findServerByName resolves current IDs and migrated definition aliases, applying
// the same visibility checks as listing. Instances are never registry entries.
func (h *Handler) findServerByName(req api.Context, serverName, reverseDNS string) (types.RegistryServerResponse, error) {
	expectedDNS, err := ReverseDNSFromURL(h.serverURL)
	if err != nil {
		return types.RegistryServerResponse{}, err
	}
	if reverseDNS != expectedDNS || system.IsVMCPInstanceID(serverName) {
		return types.RegistryServerResponse{}, h.notFoundError("Server not found")
	}
	definition, instance, err := vmcp.ResolveID(req.Context(), req.Storage, serverName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return types.RegistryServerResponse{}, h.notFoundError("Server not found")
		}
		return types.RegistryServerResponse{}, err
	}
	if definition == nil || instance != nil || !registryVMCPVisible(req, definition) {
		return types.RegistryServerResponse{}, h.notFoundError("Server not found")
	}
	return ConvertVMCPToRegistry(*definition, h.serverURL, expectedDNS), nil
}

// notFoundError returns a standard 404 error response in the format:
// {"title":"Not Found","status":404,"detail":"<message>"}
func (h *Handler) notFoundError(detail string) error {
	return &types.ErrHTTP{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf(`{"title":"Not Found","status":404,"detail":"%s"}`, detail),
	}
}
