package builtinmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SupervisorAsker answers questions about what agents are doing from the session graph.
type SupervisorAsker interface {
	AskSupervisor(ctx context.Context, question, sessionID string) (answer, model string, err error)
}

type daysInput struct {
	Days int `json:"days,omitempty" jsonschema:"How many days back to look. Defaults to 7, at most 90."`
}

type supervisorInput struct {
	Question  string `json:"question" jsonschema:"The question about agent activity, risk or policy."`
	SessionID string `json:"sessionID,omitempty" jsonschema:"A provenance session ID to focus on."`
}

type sessionsInput struct {
	State string `json:"state,omitempty" jsonschema:"Only sessions in this state, such as RUNNING or SUSPENDED."`
}

func window(days int) (time.Time, time.Time, int) {
	if days <= 0 {
		days = 7
	}
	days = min(days, 90)
	end := time.Now().UTC()
	return end.Add(-time.Duration(days) * 24 * time.Hour), end, days
}

// addInsightTools gives the admin server the tools Obot's assistant needs to answer questions
// about cost, adoption, security and operations.
func (h *Handler) addInsightTools(s *mcp.Server, req api.Context) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "llm_usage",
		Description: "LLM gateway usage and spend over recent days: totals, spend by provider, top models by " +
			"requests and by spend, and the heaviest users. Use it for cost questions.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in daysInput) (*mcp.CallToolResult, any, error) {
		start, end, days := window(in.Days)
		stats, err := req.GatewayClient.TokenUsageStats(ctx, gclient.TokenUsageStatsOptions{Start: start, End: end})
		if err != nil {
			return toolError(err)
		}
		byUser, err := req.GatewayClient.TokenUsageByUser(ctx, start, end)
		if err != nil {
			return toolError(err)
		}
		slices.SortFunc(byUser, func(a, b gtypes.RunTokenActivity) int { return b.Usage.TotalTokens - a.Usage.TotalTokens })
		users := make([]map[string]any, 0, 10)
		for _, u := range byUser[:min(len(byUser), 10)] {
			users = append(users, map[string]any{"user": h.userLabel(ctx, req, u.UserID), "totalTokens": u.Usage.TotalTokens, "totalSpendUSD": u.Usage.TotalSpend})
		}
		return jsonResult(map[string]any{
			"days":                days,
			"totals":              stats.Totals,
			"providers":           stats.Providers,
			"topModelsByRequests": stats.TopModelsByRequests,
			"topModelsBySpend":    stats.TopModelsBySpend,
			"topUsersByTokens":    users,
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "adoption_summary",
		Description: "How the organization is adopting AI through Obot over recent days: users, active users, " +
			"vMCPs and catalog size, MCP server requests, and Claude environment sessions.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in daysInput) (*mcp.CallToolResult, any, error) {
		start, end, days := window(in.Days)
		users, err := req.GatewayClient.Users(ctx, gtypes.UserQuery{})
		if err != nil {
			return toolError(err)
		}
		active, err := req.GatewayClient.ActiveUsersByDate(ctx, start, end)
		if err != nil {
			return toolError(err)
		}
		activeNames := make([]string, 0, len(active))
		for _, u := range active[:min(len(active), 25)] {
			activeNames = append(activeNames, cmpOr(u.Email, u.Username))
		}
		var (
			vmcps    v1.VMCPList
			entries  v1.MCPServerCatalogEntryList
			requests v1.MCPServerRequestList
			sessions v1.AgentInstanceList
		)
		for _, list := range []kclient.ObjectList{&vmcps, &entries, &requests, &sessions} {
			if err := req.List(list); err != nil {
				return toolError(err)
			}
		}
		requestStates := map[string]int{}
		for _, r := range requests.Items {
			if r.CreationTimestamp.After(start) {
				requestStates[cmpOr(string(r.Status.State), string(types.MCPServerRequestStatePending))]++
			}
		}
		recentSessions := 0
		sessionUsers := map[string]bool{}
		for _, s := range sessions.Items {
			if s.CreationTimestamp.After(start) {
				recentSessions++
				sessionUsers[s.Spec.UserID] = true
			}
		}
		shared := 0
		for _, v := range vmcps.Items {
			if v.Spec.UserID == "" {
				shared++
			}
		}
		return jsonResult(map[string]any{
			"days":                          days,
			"users":                         len(users),
			"activeUsers":                   len(active),
			"activeUserNames":               activeNames,
			"sharedVMCPs":                   shared,
			"personalVMCPs":                 len(vmcps.Items) - shared,
			"catalogEntries":                len(entries.Items),
			"mcpServerRequestsByState":      requestStates,
			"claudeEnvironmentSessions":     recentSessions,
			"claudeEnvironmentSessionUsers": len(sessionUsers),
		})
	})

	if h.supervisor != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name: "ask_supervisor",
			Description: "Ask Obot's AI Supervisor about agent activity across surfaces (LLM gateway, MCP gateway, " +
				"devices, egress): risky sessions, policy decisions, held actions and what to do next. Use it for " +
				"security and compliance questions.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in supervisorInput) (*mcp.CallToolResult, any, error) {
			answer, model, err := h.supervisor.AskSupervisor(ctx, in.Question, in.SessionID)
			if err != nil {
				return toolError(err)
			}
			return jsonResult(map[string]any{"answer": answer, "model": model})
		})
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_claude_sessions",
		Description: "List Claude environment sessions running on Obot's infrastructure, with their user, state and repositories.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sessionsInput) (*mcp.CallToolResult, any, error) {
		var list v1.AgentInstanceList
		if err := req.List(&list); err != nil {
			return toolError(err)
		}
		out := []map[string]any{}
		for _, s := range list.Items {
			if in.State != "" && !strings.EqualFold(s.Status.State, in.State) {
				continue
			}
			var repos []string
			for _, r := range s.Spec.ClaudeRepositories {
				repos = append(repos, strings.TrimSuffix(fmt.Sprintf("%s@%s", r.URL, r.Revision), "@"))
			}
			out = append(out, map[string]any{
				"id":           s.Name,
				"user":         h.userLabel(ctx, req, s.Spec.UserID),
				"environment":  s.Spec.DisplayName,
				"state":        s.Status.State,
				"error":        s.Status.Error,
				"created":      s.CreationTimestamp.UTC().Format(time.RFC3339),
				"repositories": repos,
			})
		}
		return jsonResult(map[string]any{"sessions": out})
	})
}

func (h *Handler) userLabel(ctx context.Context, req api.Context, id string) string {
	if id == "" {
		return ""
	}
	if u, err := req.GatewayClient.UserByID(ctx, id); err == nil {
		return cmpOr(u.Email, u.Username, id)
	}
	return id
}
