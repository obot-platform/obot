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
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	serverURL   string
	mimeFetcher *mimeFetcher
}

func NewHandler(serverURL string, egress mcp.RemoteMCPURLValidationConfig) *Handler {
	return &Handler{
		serverURL:   serverURL,
		mimeFetcher: newMimeFetcher(egress),
	}
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

	servers := make([]types.RegistryServerResponse, 0, len(list.Items))
	for _, item := range list.Items {
		if registryVMCPVisible(req.User, &item) {
			servers = append(servers, ConvertVMCPToRegistry(req.Context(), item, h.serverURL, reverseDNS, h.mimeFetcher))
		}
	}

	if search := req.URL.Query().Get("search"); search != "" {
		servers = filterServersBySearch(servers, search)
	}
	return req.Write(paginateServers(servers, req.URL.Query().Get("cursor"), parseLimit(req.URL.Query().Get("limit"))))
}

func registryUserAuthenticated(u user.Info) bool {
	return u != nil && !slices.Contains(u.GetGroups(), authz.UnauthenticatedGroup)
}

// Registry visibility follows consumption permissions, including for administrators.
// Anonymous discovery deliberately publishes only shared definitions with an explicit
// wildcard profile, without interpreting anonymous user IDs or groups as grants.
func registryVMCPVisible(u user.Info, definition *v1.VMCP) bool {
	if registryUserAuthenticated(u) {
		return authz.UserCanReadVMCP(u, definition)
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

func filterServersBySearch(servers []types.RegistryServerResponse, search string) []types.RegistryServerResponse {
	search = strings.ToLower(search)
	var result []types.RegistryServerResponse

	for _, server := range servers {
		// Substring match on name, title, or description
		if strings.Contains(strings.ToLower(server.Server.Name), search) ||
			strings.Contains(strings.ToLower(server.Server.Title), search) ||
			strings.Contains(strings.ToLower(server.Server.Description), search) {
			result = append(result, server)
		}
	}

	return result
}

func paginateServers(servers []types.RegistryServerResponse, cursor string, limit int) types.RegistryServerList {
	// Sort servers by creation timestamp
	slices.SortStableFunc(servers, func(i, j types.RegistryServerResponse) int {
		if result := cmp.Compare(i.CreatedAtUnix, j.CreatedAtUnix); result != 0 {
			return result
		}
		return strings.Compare(i.Server.Name, j.Server.Name)
	})

	// Simple cursor-based pagination using server name as cursor
	startIdx := 0
	if cursor != "" {
		// Find the position after the cursor
		for i, server := range servers {
			if server.Server.Name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get the page
	endIdx := startIdx + limit
	endIdx = min(endIdx, len(servers))

	page := servers[startIdx:endIdx]
	if page == nil {
		// This prevents an error from showing up in VSCode when the result is empty.
		page = []types.RegistryServerResponse{}
	}

	// Build response
	response := types.RegistryServerList{
		Servers: page,
	}

	// Add metadata if there are more results
	if endIdx < len(servers) {
		response.Metadata = &types.RegistryServerListMetadata{
			NextCursor: page[len(page)-1].Server.Name,
			Count:      len(page),
		}
	} else {
		response.Metadata = &types.RegistryServerListMetadata{
			Count: len(page),
		}
	}

	return response
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
		return h.notFoundError("Server not found")
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
		return h.notFoundError("Server not found")
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
		return types.RegistryServerResponse{}, err
	}
	if definition == nil || instance != nil || !registryVMCPVisible(req.User, definition) {
		return types.RegistryServerResponse{}, h.notFoundError("Server not found")
	}
	return ConvertVMCPToRegistry(req.Context(), *definition, h.serverURL, expectedDNS, h.mimeFetcher), nil
}

// notFoundError returns a standard 404 error response in the format:
// {"title":"Not Found","status":404,"detail":"<message>"}
func (h *Handler) notFoundError(detail string) error {
	return &types.ErrHTTP{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf(`{"title":"Not Found","status":404,"detail":"%s"}`, detail),
	}
}
