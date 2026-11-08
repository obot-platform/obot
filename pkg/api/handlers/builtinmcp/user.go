package builtinmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	userInstructions = `Obot is your organization's gateway for MCP servers. Use these tools to find MCP servers ` +
		`your organization already provides before installing one yourself, and to ask an administrator to add one ` +
		`that isn't available yet. When the user wants a server Obot doesn't have, or one they found on the web, ` +
		`call request_mcp_server instead of configuring the server locally. Connect to an Obot vMCP at its connectURL.`
)

type searchInput struct {
	Query string `json:"query,omitempty" jsonschema:"Words to look for in server names and descriptions. Leave empty to list everything available."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results. Defaults to 20."`
}

type vmcpRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ConnectURL string `json:"connectURL"`
}

type searchResult struct {
	// Kind is "vmcp", a server you can connect to, or "catalogEntry", a server in the
	// catalog, which you reach through a vMCP that includes it.
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ConnectURL  string    `json:"connectURL,omitempty"`
	Servers     []string  `json:"servers,omitempty"`
	Runtime     string    `json:"runtime,omitempty"`
	InVMCPs     []vmcpRef `json:"inVMCPs,omitempty"`
}

type requestInput struct {
	Name            string                         `json:"name" jsonschema:"The server's name."`
	Description     string                         `json:"description,omitempty" jsonschema:"What the server does."`
	Reason          string                         `json:"reason,omitempty" jsonschema:"Why the user wants it, in their words where possible. Administrators read this."`
	SourceURL       string                         `json:"sourceURL,omitempty" jsonschema:"Where the server was found, such as its repository or documentation page."`
	Runtime         string                         `json:"runtime" jsonschema:"How the server runs: remote (a hosted MCP endpoint), npx (an npm package), uvx (a PyPI package) or containerized (a container image)."`
	RemoteURL       string                         `json:"remoteURL,omitempty" jsonschema:"For remote: the MCP endpoint URL."`
	Package         string                         `json:"package,omitempty" jsonschema:"For npx or uvx: the package name."`
	Image           string                         `json:"image,omitempty" jsonschema:"For containerized: the image."`
	Port            int                            `json:"port,omitempty" jsonschema:"For containerized: the port the MCP endpoint listens on."`
	Path            string                         `json:"path,omitempty" jsonschema:"For containerized: the HTTP path of the MCP endpoint, such as /mcp."`
	Args            []string                       `json:"args,omitempty" jsonschema:"Arguments the server needs."`
	Config          []types.MCPServerRequestConfig `json:"config,omitempty" jsonschema:"Values each user must supply, such as API keys, by environment variable or header name. Never include the values themselves."`
	SuggestedVMCPID string                         `json:"suggestedVMCPID,omitempty" jsonschema:"The ID of an existing vMCP the user would like the server added to."`
}

type requestStateInput struct {
	State string `json:"state,omitempty" jsonschema:"Only list requests in this state: pending, approved or denied."`
}

type requestIDInput struct {
	ID string `json:"id" jsonschema:"The request's ID."`
}

type requestView struct {
	types.MCPServerRequest
	ReviewURL  string `json:"reviewURL,omitempty"`
	ConnectURL string `json:"connectURL,omitempty"`
}

func (h *Handler) userServer(req api.Context) *mcp.Server {
	s := newServer("obot", "Obot", userInstructions)
	h.addUserTools(s, req)
	return s
}

func (h *Handler) addUserTools(s *mcp.Server, req api.Context) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_mcp_servers",
		Description: "Search the MCP servers available to the user in Obot: vMCPs they can connect to and servers in the catalog.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		results, err := h.search(req, in.Query, in.Limit)
		if err != nil {
			return toolError(err)
		}
		if len(results) == 0 {
			return jsonResult(map[string]any{
				"results": results,
				"hint":    "Nothing matched. If the user wants a server Obot doesn't have, use request_mcp_server.",
			})
		}
		return jsonResult(map[string]any{"results": results})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "request_mcp_server",
		Description: "Ask an Obot administrator to add an MCP server to the organization's catalog. " +
			"Use it for a server the user found on the web or one search_mcp_servers didn't find. " +
			"An administrator is alerted and approves or denies the request; check on it with get_mcp_server_request.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: new(false)},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in requestInput) (*mcp.CallToolResult, any, error) {
		request, err := h.requests.CreateRequest(req, types.MCPServerRequestManifest{
			Name:            in.Name,
			Description:     in.Description,
			Reason:          in.Reason,
			SourceURL:       in.SourceURL,
			Runtime:         types.Runtime(strings.ToLower(strings.TrimSpace(in.Runtime))),
			RemoteURL:       in.RemoteURL,
			Package:         in.Package,
			Image:           in.Image,
			Port:            in.Port,
			Path:            in.Path,
			Args:            in.Args,
			Config:          in.Config,
			SuggestedVMCPID: in.SuggestedVMCPID,
		})
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{
			"request": h.view(request),
			"message": "Request submitted. An Obot administrator will approve or deny it.",
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_my_mcp_server_requests",
		Description: "List the user's requests for new MCP servers and where each stands.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in requestStateInput) (*mcp.CallToolResult, any, error) {
		requests, err := h.requests.ListRequests(req, false, types.MCPServerRequestState(in.State))
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{"requests": h.views(requests)})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_mcp_server_request",
		Description: "Get one MCP server request. Once approved, it includes the vMCP to connect to.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in requestIDInput) (*mcp.CallToolResult, any, error) {
		request, err := h.requests.GetRequest(req, in.ID)
		if err != nil {
			return toolError(err)
		}
		return jsonResult(h.view(request))
	})
}

func (h *Handler) view(r types.MCPServerRequest) requestView {
	v := requestView{MCPServerRequest: r}
	if r.State == types.MCPServerRequestStatePending {
		v.ReviewURL = h.requests.ReviewURL(r.ID)
	}
	if r.VMCPID != "" {
		v.ConnectURL = h.connectURL(r.VMCPID)
	}
	return v
}

func (h *Handler) views(requests []types.MCPServerRequest) []requestView {
	views := make([]requestView, 0, len(requests))
	for _, r := range requests {
		views = append(views, h.view(r))
	}
	return views
}

// search finds the vMCPs the user can connect to and the catalog entries they can see
// whose names or descriptions contain every word of query.
func (h *Handler) search(req api.Context, query string, limit int) ([]searchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	terms := strings.Fields(strings.ToLower(query))
	matches := func(texts ...string) bool {
		haystack := strings.ToLower(strings.Join(texts, " "))
		for _, t := range terms {
			if !strings.Contains(haystack, t) {
				return false
			}
		}
		return true
	}

	var vmcps v1.VMCPList
	if err := req.List(&vmcps); err != nil {
		return nil, err
	}
	var results []searchResult
	// vMCPs the user can connect to, by catalog entry they include.
	byEntry := map[string][]vmcpRef{}
	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		if !authz.UserCanConnectVMCP(req.User, vmcp) {
			continue
		}
		ref := vmcpRef{ID: vmcp.Name, Name: vmcp.Spec.Manifest.DisplayName, ConnectURL: h.connectURL(vmcp.Name)}
		texts := []string{vmcp.Spec.Manifest.DisplayName, vmcp.Spec.Manifest.Description}
		var servers []string
		for _, c := range vmcp.Spec.Manifest.Components {
			byEntry[c.MCPServerCatalogEntryID] = append(byEntry[c.MCPServerCatalogEntryID], ref)
			name := cmpOr(c.CatalogEntry.Manifest.Name, c.Name)
			servers = append(servers, name)
			texts = append(texts, name, c.CatalogEntry.Manifest.ShortDescription)
		}
		if matches(texts...) {
			results = append(results, searchResult{
				Kind:        "vmcp",
				ID:          vmcp.Name,
				Name:        vmcp.Spec.Manifest.DisplayName,
				Description: vmcp.Spec.Manifest.Description,
				ConnectURL:  ref.ConnectURL,
				Servers:     servers,
			})
		}
	}

	var entries v1.MCPServerCatalogEntryList
	if err := req.List(&entries); err != nil {
		return nil, err
	}
	for _, entry := range entries.Items {
		m := entry.Spec.Manifest
		if !matches(m.Name, m.ShortDescription, m.Description) {
			continue
		}
		var (
			allowed bool
			err     error
		)
		switch {
		case entry.Spec.MCPCatalogName != "":
			allowed, err = h.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, entry.Name, entry.Spec.MCPCatalogName)
		case entry.Spec.PowerUserWorkspaceID != "":
			allowed, err = h.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, entry.Name, entry.Spec.PowerUserWorkspaceID)
		}
		if err != nil {
			return nil, fmt.Errorf("check access to %s: %w", entry.Name, err)
		}
		if !allowed && !req.UserIsAdmin() {
			continue
		}
		results = append(results, searchResult{
			Kind:        "catalogEntry",
			ID:          entry.Name,
			Name:        m.Name,
			Description: cmpOr(m.ShortDescription, m.Description),
			Runtime:     string(m.Runtime),
			InVMCPs:     byEntry[entry.Name],
		})
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
