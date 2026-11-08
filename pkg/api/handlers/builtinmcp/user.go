package builtinmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
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
	// Note tells the agent what to do with a catalog entry it can't reach yet.
	Note  string `json:"note,omitempty"`
	score int
}

type requestInput struct {
	Name                   string                         `json:"name,omitempty" jsonschema:"The server's name. Not needed with existingCatalogEntryID."`
	Description            string                         `json:"description,omitempty" jsonschema:"What the server does."`
	Reason                 string                         `json:"reason,omitempty" jsonschema:"Why the user wants it, in their words where possible. Administrators read this."`
	SourceURL              string                         `json:"sourceURL,omitempty" jsonschema:"Where the server was found, such as its repository or documentation page."`
	ExistingCatalogEntryID string                         `json:"existingCatalogEntryID,omitempty" jsonschema:"To ask for a server that is already in Obot's catalog but in no vMCP the user can connect to, its catalog entry ID from search_mcp_servers. The fields describing the server are then not needed."`
	Runtime                string                         `json:"runtime,omitempty" jsonschema:"How a new server runs: remote (a hosted MCP endpoint), npx (an npm package), uvx (a PyPI package) or containerized (a container image)."`
	RemoteURL              string                         `json:"remoteURL,omitempty" jsonschema:"For remote: the MCP endpoint URL."`
	Package                string                         `json:"package,omitempty" jsonschema:"For npx or uvx: the package name."`
	Image                  string                         `json:"image,omitempty" jsonschema:"For containerized: the image."`
	Port                   int                            `json:"port,omitempty" jsonschema:"For containerized: the port the MCP endpoint listens on."`
	Path                   string                         `json:"path,omitempty" jsonschema:"For containerized: the HTTP path of the MCP endpoint, such as /mcp."`
	Args                   []string                       `json:"args,omitempty" jsonschema:"Arguments the server needs."`
	Config                 []types.MCPServerRequestConfig `json:"config,omitempty" jsonschema:"Values each user must supply, such as API keys, by environment variable or header name. Never include the values themselves."`
	SuggestedVMCPID        string                         `json:"suggestedVMCPID,omitempty" jsonschema:"The ID of an existing vMCP the user would like the server added to."`
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
		Description: "Ask an Obot administrator for an MCP server. For a server Obot doesn't have, such as one the user " +
			"found on the web, describe it. For one search_mcp_servers found in the catalog but in no vMCP the user can " +
			"connect to, pass its ID as existingCatalogEntryID. An administrator is alerted and approves or denies the request; check " +
			"on it with get_mcp_server_request.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: new(false)},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in requestInput) (*mcp.CallToolResult, any, error) {
		request, err := h.requests.CreateRequest(req, types.MCPServerRequestManifest{
			Name:                   in.Name,
			Description:            in.Description,
			Reason:                 in.Reason,
			SourceURL:              in.SourceURL,
			Runtime:                types.Runtime(strings.ToLower(strings.TrimSpace(in.Runtime))),
			RemoteURL:              in.RemoteURL,
			Package:                in.Package,
			Image:                  in.Image,
			Port:                   in.Port,
			Path:                   in.Path,
			Args:                   in.Args,
			Config:                 in.Config,
			SuggestedVMCPID:        in.SuggestedVMCPID,
			ExistingCatalogEntryID: strings.TrimSpace(in.ExistingCatalogEntryID),
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

// search finds the vMCPs the user can connect to and the catalog entries they can see that
// match every word of query at the start of a word, best matches first: an exact name, then
// names containing every word, then descriptions.
func (h *Handler) search(req api.Context, query string, limit int) ([]searchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := normalize(query)
	terms := strings.Fields(q)

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
		descriptions := []string{vmcp.Spec.Manifest.Description}
		var servers []string
		granted := grantedComponents(req.User, vmcp)
		for _, c := range vmcp.Spec.Manifest.Components {
			// Only the servers the user's profiles grant are theirs to use.
			if !granted(c.ID) {
				continue
			}
			byEntry[c.MCPServerCatalogEntryID] = append(byEntry[c.MCPServerCatalogEntryID], ref)
			name := cmpOr(c.CatalogEntry.Manifest.Name, c.Name)
			servers = append(servers, name)
			descriptions = append(descriptions, name, c.CatalogEntry.Manifest.ShortDescription)
		}
		if score := matchScore(q, terms, vmcp.Spec.Manifest.DisplayName, descriptions...); score > 0 {
			results = append(results, searchResult{
				Kind:        "vmcp",
				ID:          vmcp.Name,
				Name:        vmcp.Spec.Manifest.DisplayName,
				Description: vmcp.Spec.Manifest.Description,
				ConnectURL:  ref.ConnectURL,
				Servers:     servers,
				// Something the user can use now ranks above a catalog entry that matches as well.
				score: score + 1,
			})
		}
	}

	var entries v1.MCPServerCatalogEntryList
	if err := req.List(&entries); err != nil {
		return nil, err
	}
	for _, entry := range entries.Items {
		m := entry.Spec.Manifest
		score := matchScore(q, terms, m.Name, m.ShortDescription, m.Description)
		if score == 0 {
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
		result := searchResult{
			Kind:        "catalogEntry",
			ID:          entry.Name,
			Name:        m.Name,
			Description: cmpOr(m.ShortDescription, m.Description),
			Runtime:     string(m.Runtime),
			InVMCPs:     byEntry[entry.Name],
			score:       score,
		}
		if len(result.InVMCPs) == 0 {
			result.Note = "In the catalog but in no vMCP you can connect to. To use it, request it with request_mcp_server, passing this ID as existingCatalogEntryID."
		}
		results = append(results, result)
	}

	slices.SortStableFunc(results, func(a, b searchResult) int { return b.score - a.score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// grantedComponents reports which of vmcp's components the user's profiles grant.
func grantedComponents(u kuser.Info, vmcp *v1.VMCP) func(string) bool {
	if vmcp.Spec.UserID != "" {
		return func(string) bool { return vmcp.Spec.UserID == u.GetUID() }
	}
	ids := map[string]bool{}
	for _, p := range vmcpaccess.MatchingProfiles(u, vmcp.Spec.Manifest.Profiles) {
		if p.Permissions.AllowAllComponents {
			return func(string) bool { return true }
		}
		for id := range p.Permissions.AllowedComponents {
			ids[id] = true
		}
	}
	return func(id string) bool { return ids[id] }
}

// normalize lowercases s and turns everything but letters and digits into spaces.
func normalize(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// hasWordPrefix reports whether a word of text starts with term.
func hasWordPrefix(text, term string) bool {
	for w := range strings.FieldsSeq(text) {
		if strings.HasPrefix(w, term) {
			return true
		}
	}
	return false
}

// matchScore ranks how well name and descriptions match a normalized query. Zero means no
// match; an empty query matches everything weakly.
func matchScore(query string, terms []string, name string, descriptions ...string) int {
	if len(terms) == 0 {
		return 1
	}
	name = normalize(name)
	description := normalize(strings.Join(descriptions, " "))
	inName := 0
	for _, t := range terms {
		switch {
		case hasWordPrefix(name, t):
			inName++
		case hasWordPrefix(description, t):
		default:
			return 0
		}
	}
	switch {
	case name == query:
		return 100
	case strings.HasPrefix(name, query):
		return 80
	case inName == len(terms):
		return 60
	case inName > 0:
		return 30 + inName
	}
	return 10
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
