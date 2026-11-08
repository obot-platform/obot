package builtinmcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	adminInstructions = `Tools for Obot administrators. Review users' requests for new MCP servers and approve ` +
		`them into a vMCP or deny them. Before approving, look at the request with get_mcp_server_request and ` +
		`confirm the destination with the administrator: an existing shared vMCP (find one with list_shared_vmcps) ` +
		`or a new vMCP. Approving adds the server to the organization's catalog and makes it available to everyone ` +
		`the vMCP grants access to.`
)

type adminListInput struct {
	State string `json:"state,omitempty" jsonschema:"Only list requests in this state: pending (the default), approved, denied, or all."`
}

type vmcpListInput struct {
	Query string `json:"query,omitempty" jsonschema:"Words to look for in vMCP names and descriptions."`
}

type approveInput struct {
	ID                 string `json:"id" jsonschema:"The request's ID."`
	VMCPID             string `json:"vmcpID,omitempty" jsonschema:"Add the server to this existing shared vMCP. Give either this or newVMCPName."`
	NewVMCPName        string `json:"newVMCPName,omitempty" jsonschema:"Create a vMCP with this name for the server. Give either this or vmcpID."`
	NewVMCPDescription string `json:"newVMCPDescription,omitempty" jsonschema:"A description for the new vMCP."`
	Reason             string `json:"reason,omitempty" jsonschema:"A note for the requester."`
}

type denyInput struct {
	ID     string `json:"id" jsonschema:"The request's ID."`
	Reason string `json:"reason,omitempty" jsonschema:"Why the request is denied. The requester sees this."`
}

type vmcpSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Servers     []string `json:"servers,omitempty"`
}

func (h *Handler) adminServer(req api.Context) *mcp.Server {
	s := newServer("obot-admin", "Obot Admin", adminInstructions)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_mcp_server_requests",
		Description: "List users' requests for new MCP servers, pending ones by default.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in adminListInput) (*mcp.CallToolResult, any, error) {
		state := types.MCPServerRequestState(strings.ToLower(in.State))
		switch state {
		case "":
			state = types.MCPServerRequestStatePending
		case "all":
			state = ""
		}
		requests, err := h.requests.ListRequests(req, true, state)
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{"requests": h.views(requests)})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_mcp_server_request",
		Description: "Get one MCP server request, including the catalog entry approving it would create.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in requestIDInput) (*mcp.CallToolResult, any, error) {
		request, err := h.requests.GetRequest(req, in.ID)
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{
			"request":              h.view(request),
			"proposedCatalogEntry": request.CatalogEntryManifest(),
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_shared_vmcps",
		Description: "List the shared vMCPs an approved server can be added to.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in vmcpListInput) (*mcp.CallToolResult, any, error) {
		var list v1.VMCPList
		if err := req.List(&list); err != nil {
			return toolError(err)
		}
		terms := strings.Fields(strings.ToLower(in.Query))
		summaries := []vmcpSummary{}
	next:
		for _, vmcp := range list.Items {
			if vmcp.Spec.UserID != "" || vmcp.Spec.SourceURL != "" {
				continue
			}
			s := vmcpSummary{ID: vmcp.Name, Name: vmcp.Spec.Manifest.DisplayName, Description: vmcp.Spec.Manifest.Description}
			for _, c := range vmcp.Spec.Manifest.Components {
				s.Servers = append(s.Servers, cmpOr(c.CatalogEntry.Manifest.Name, c.Name))
			}
			haystack := strings.ToLower(s.Name + " " + s.Description + " " + strings.Join(s.Servers, " "))
			for _, t := range terms {
				if !strings.Contains(haystack, t) {
					continue next
				}
			}
			summaries = append(summaries, s)
		}
		return jsonResult(map[string]any{"vmcps": summaries})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "approve_mcp_server_request",
		Description: "Approve a request: add the server to the catalog and to an existing shared vMCP (vmcpID) " +
			"or a new vMCP (newVMCPName). Confirm the destination with the administrator first.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), IdempotentHint: false},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in approveInput) (*mcp.CallToolResult, any, error) {
		approval := types.MCPServerRequestApproval{VMCPID: strings.TrimSpace(in.VMCPID), Reason: in.Reason}
		if name := strings.TrimSpace(in.NewVMCPName); name != "" {
			approval.NewVMCP = &types.MCPServerRequestNewVMCP{DisplayName: name, Description: in.NewVMCPDescription}
		}
		request, err := h.requests.ApproveRequest(req, in.ID, approval)
		if err != nil {
			return toolError(err)
		}
		message := "Approved."
		if request.RequesterCanConnect != nil && !*request.RequesterCanConnect {
			message += " The requester can't connect to this vMCP yet: no profile grants them access. Add them to one of its profiles in Obot."
		}
		return jsonResult(map[string]any{"request": h.view(request), "message": message})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "deny_mcp_server_request",
		Description: "Deny a request. The requester sees the reason.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in denyInput) (*mcp.CallToolResult, any, error) {
		request, err := h.requests.DenyRequest(req, in.ID, in.Reason)
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{"request": h.view(request), "message": "Denied."})
	})

	// Administrators can search too.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_mcp_servers",
		Description: "Search vMCPs and catalog entries in Obot.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		results, err := h.search(req, in.Query, in.Limit)
		if err != nil {
			return toolError(err)
		}
		return jsonResult(map[string]any{"results": results})
	})
	return s
}
