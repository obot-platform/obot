package authz

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MetricsGroup         = "metrics"
	UnauthenticatedGroup = "unauthenticated"

	// scimPathPrefix begins the path of the SCIM endpoint, which serves the SCIM connection. It must match the SCIM
	// handler's path prefix.
	scimPathPrefix = "/scim/v2/"

	// anyGroup is an internal group that allows access to any group
	anyGroup = "*"
)

var (
	tunnelResources       = newPathMatcher("GET /tunnel/connect")
	tunnelBridgeResources = newPathMatcher(
		"/tunnel/bridge/{target}",
		"GET /tunnel/composite/register/{key}",
	)
	tunnelPeerResources = newPathMatcher("GET /tunnel/peer")

	adminAndOwnerRules = []string{
		"/api/registry-settings",
		"/api/claude-organizations/",
		"/api/claude-environments",
		"/api/claude-environments/",
		"POST /api/openapi-import",
		"/api/model-proxy",
		"/api/model-proxy/",
		"/api/mcp-tunnels",
		"/api/mcp-tunnels/",
		"/api/mcp-catalogs",
		"/api/mcp-catalogs/",
		"GET /api/mcp-server-binding-secrets",
		"/api/mcp-servers",
		"/api/mcp-servers/",
		"/api/vmcps",
		"/api/vmcps/",
		"/api/vmcp-instances",
		"/api/vmcp-instances/",
		"/api/all-mcps",
		"/api/all-mcps/",
		"/api/workspaces",
		"/api/workspaces/",
		"/api/mcp-webhook-validations",
		"/api/mcp-webhook-validations/",
		"/api/system-mcp-servers",
		"/api/system-mcp-servers/",
		"/api/system-mcp-catalogs",
		"/api/system-mcp-catalogs/",
		"GET /api/mcp-audit-logs",
		"GET /api/mcp-audit-logs/",
		"GET /api/egress-audit-logs",
		"GET /api/egress-audit-logs/",
		"GET /api/llm-audit-logs",
		"GET /api/llm-audit-logs/",
		"GET /api/llm-audit-logs/filter-options/",
		"GET /api/mcp-stats",
		"GET /api/mcp-stats/",
		"GET /debug/pprof/",
		"GET /debug/triggers",
		"GET /debug/metrics",
		"PUT /api/license",
		"POST /api/license",
		"POST /api/license/community",
		"DELETE /api/license",
		"POST /api/auth-providers/{id}/configure",
		"POST /api/auth-providers/{id}/deconfigure",
		"POST /api/auth-providers/{id}/reveal",
		"GET /api/auth-providers/{id}/residual-group-data",
		"GET /api/scim-connections",
		"GET /api/scim-connections/",
		"/api/local-auth/users",
		"/api/local-auth/users/",
		"/api/model-providers",
		"/api/model-providers/",
		"/api/llm-gateway/",
		"GET /api/bookstrap",
		"/api/models",
		"/api/models/",
		"/api/model-access-policies",
		"/api/model-access-policies/",
		"DELETE /api/devices/scans/{scan_id}",
		"/api/devices/scan-stats",
		"/api/devices/mcp-servers/",
		"/api/devices/skills",
		"/api/devices/skills/",
		"/api/devices/clients",
		"/api/devices/clients/",
		"/api/mdm/configurations",
		"/api/mdm/configurations/",
		"GET /api/mdm/devices",
		"DELETE /api/mdm/devices/",
		"GET /api/enforcement-decisions",
		"GET /api/enforcement-decisions/",
		"/api/mdm/asset-source",
		"/api/mdm/asset-source/",
		"GET /api/mdm/assets",
		"/api/available-models",
		"/api/available-models/",
		"/api/default-model-aliases",
		"/api/default-model-aliases/",
		"/api/users",
		"POST /api/users/{user_id}/enable",
		"GET /api/groups",
		"/api/group-role-assignments",
		"/api/group-role-assignments/",
		"POST /api/encrypt-all-users",
		"GET /api/active-users",
		"GET /api/token-usage",
		"GET /api/total-token-usage",
		"GET /api/token-usage-stats",
		"GET /api/tokens",
		"DELETE /api/tokens/{id}",
		"/api/oauth-apps",
		"/api/oauth-apps/",
		"/api/user-default-role-settings",
		"/api/setup/",
		"/api/k8s-settings",
		"/api/image-pull-secrets",
		"/api/image-pull-secrets/",
		"/api/git-credentials",
		"/api/git-credentials/",
		"/api/mcp-capacity",
		"/api/audit-log-exports",
		"/api/audit-log-exports/",
		"/api/scheduled-audit-log-exports",
		"/api/scheduled-audit-log-exports/",
		"/api/storage-credentials",
		"/api/storage-credentials/",
		"/api/oauth-clients",
		"/api/oauth-clients/",
		"/api/skill-repositories",
		"/api/skill-repositories/",
		"/api/skill-access-rules",
		"/api/skill-access-rules/",
		"GET /api/eula",
		"PUT /api/eula",
		"/api/product-telemetry-consent",
		"PUT /api/app-preferences",
		"PUT /api/app-notification",

		// Allow admins to upload custom images
		"POST /api/image/upload",

		// Admin API key management endpoints
		"GET /api/admin-api-keys",
		"GET /api/admin-api-keys/{id}",
		"DELETE /api/admin-api-keys/{id}",
		"POST /api/claude-sessions/{session_id}/suspend",
	}
	// adminReadonlyRules are what the admin read-only group may do: read the admin area and change nothing. Owners and
	// admins are also in this group; auditors are not. Every rule is a GET. Reveal endpoints, actions that are reads in
	// name only (tests, tool previews, the MCP tester), MDM installer downloads, setup and debug endpoints are
	// deliberately absent. So are GETs that resolve a connection to a server: tool, resource and prompt discovery and
	// OAuth URL and status lookups can launch a server or create the caller's connection to it. A static grant skips
	// the resource checks, so the routes that hold such GETs are granted one by one rather than as a subtree; without
	// a grant they fall back to the access any user has. This holds only for this group's rules: a user who also holds
	// another group gets what that group's rules grant, and the auditor rules still grant some of these routes.
	// TestAdminReadonlyRulesCoverAdminReads keeps this list in step with adminAndOwnerRules.
	adminReadonlyRules = []string{
		"GET /api/registry-settings",
		"GET /api/claude-organizations/",
		"GET /api/claude-environments",
		"GET /api/claude-environments/",
		"GET /api/model-proxy",
		"GET /api/model-proxy/",
		"GET /api/mcp-tunnels",
		"GET /api/mcp-tunnels/",
		"GET /api/mcp-catalogs",
		"GET /api/mcp-catalogs/{catalog_id}",
		"GET /api/mcp-catalogs/{catalog_id}/categories",
		"GET /api/mcp-catalogs/{catalog_id}/orphaned-vmcps",
		"GET /api/mcp-catalogs/{catalog_id}/entries",
		"GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}",
		"GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/servers",
		"GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/mcp-capacity",
		"GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/servers/{mcp_server_id}/k8s-settings-status",
		"GET /api/mcp-catalogs/{catalog_id}/entries/all-servers",
		"GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials",
		"GET /api/mcp-catalogs/{catalog_id}/servers",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/details",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/logs",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/instances",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/k8s-settings-status",
		"GET /api/mcp-catalogs/{catalog_id}/servers-needing-k8s-update",
		"GET /api/mcp-catalogs/{catalog_id}/servers/all-instances",
		"GET /api/mcp-catalogs/{catalog_id}/access-control-rules",
		"GET /api/mcp-catalogs/{catalog_id}/access-control-rules/{access_control_rule_id}",
		"GET /api/mcp-server-binding-secrets",
		"GET /api/mcp-servers",
		"GET /api/mcp-servers/{mcp_server_id}",
		"GET /api/mcp-servers/{mcp_server_id}/details",
		"GET /api/mcp-servers/{mcp_server_id}/logs",
		"GET /api/vmcps",
		"GET /api/vmcps/{vmcp_id}",
		"GET /api/vmcp-instances",
		"GET /api/vmcp-instances/",
		"GET /api/all-mcps",
		"GET /api/all-mcps/entries",
		"GET /api/all-mcps/entries/{entry_id}",
		"GET /api/all-mcps/servers",
		"GET /api/all-mcps/servers/{mcp_server_id}",
		"GET /api/workspaces",
		"GET /api/workspaces/{workspace_id}",
		"GET /api/workspaces/all-entries",
		"GET /api/workspaces/all-servers",
		"GET /api/workspaces/all-entries/all-servers",
		"GET /api/workspaces/all-access-control-rules",
		"GET /api/workspaces/all-servers/all-instances",
		"GET /api/workspaces/{workspace_id}/access-control-rules",
		"GET /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
		"GET /api/workspaces/{workspace_id}/entries",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/details",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/logs",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/k8s-settings-status",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials",
		"GET /api/workspaces/{workspace_id}/servers",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/details",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/logs",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/k8s-settings-status",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/instances",
		"GET /api/workspaces/servers-needing-k8s-update",
		"GET /api/mcp-webhook-validations",
		"GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}",
		"GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}/details",
		"GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}/logs",
		"GET /api/system-mcp-servers",
		"GET /api/system-mcp-servers/{id}",
		"GET /api/system-mcp-servers/{id}/details",
		"GET /api/system-mcp-servers/{id}/logs",
		"GET /api/system-mcp-catalogs",
		"GET /api/system-mcp-catalogs/",
		"GET /api/mcp-audit-logs",
		"GET /api/mcp-audit-logs/",
		"GET /api/egress-audit-logs",
		"GET /api/egress-audit-logs/",
		"GET /api/llm-audit-logs",
		"GET /api/llm-audit-logs/",
		"GET /api/mcp-stats",
		"GET /api/mcp-stats/",
		"GET /api/auth-providers/{id}/residual-group-data",
		"GET /api/scim-connections",
		"GET /api/scim-connections/",
		"GET /api/local-auth/users",
		"GET /api/local-auth/users/",
		"GET /api/model-providers",
		"GET /api/model-providers/",
		"GET /api/llm-gateway/",
		"GET /api/models",
		"GET /api/models/",
		"GET /api/model-access-policies",
		"GET /api/model-access-policies/",
		"GET /api/devices/scan-stats",
		"GET /api/devices/mcp-servers/",
		"GET /api/devices/skills",
		"GET /api/devices/skills/",
		"GET /api/devices/clients",
		"GET /api/devices/clients/",
		"GET /api/mdm/configurations",
		"GET /api/mdm/configurations/{id}",
		"GET /api/mdm/configurations/{id}/enrollment-keys",
		"GET /api/mdm/configurations/{id}/devices",
		"GET /api/mdm/devices",
		"GET /api/enforcement-decisions",
		"GET /api/enforcement-decisions/",
		"GET /api/mdm/asset-source",
		"GET /api/mdm/asset-source/",
		"GET /api/mdm/assets",
		"GET /api/available-models",
		"GET /api/available-models/",
		"GET /api/default-model-aliases",
		"GET /api/default-model-aliases/",
		"GET /api/users",
		"GET /api/users/",
		"GET /api/groups",
		"GET /api/groups/",
		"GET /api/group-role-assignments",
		"GET /api/group-role-assignments/",
		"GET /api/active-users",
		"GET /api/token-usage",
		"GET /api/total-token-usage",
		"GET /api/token-usage-stats",
		"GET /api/tokens",
		"GET /api/oauth-apps",
		"GET /api/oauth-apps/",
		"GET /api/user-default-role-settings",
		"GET /api/k8s-settings",
		"GET /api/image-pull-secrets",
		"GET /api/image-pull-secrets/",
		"GET /api/git-credentials",
		"GET /api/git-credentials/",
		"GET /api/mcp-capacity",
		"GET /api/audit-log-exports",
		"GET /api/audit-log-exports/",
		"GET /api/scheduled-audit-log-exports",
		"GET /api/scheduled-audit-log-exports/",
		"GET /api/storage-credentials",
		"GET /api/storage-credentials/",
		"GET /api/oauth-clients",
		"GET /api/oauth-clients/",
		"GET /api/skill-repositories",
		"GET /api/skill-repositories/",
		"GET /api/skill-access-rules",
		"GET /api/skill-access-rules/",
		"GET /api/eula",
		"GET /api/product-telemetry-consent",
		"GET /api/admin-api-keys",
		"GET /api/admin-api-keys/{id}",
	}
	ownerRules = []string{
		"POST /api/auth-providers/{id}/stage",
		"DELETE /api/auth-providers/{id}/stage",
		"POST /api/auth-providers/{id}/verify",
		"POST /api/auth-providers/{id}/activate",
		"POST /api/scim-connections",
		"POST /api/scim-connections/{id}/enforce",
		"POST /api/scim-connections/{id}/delete-unreferenced-groups",
		"POST /api/scim-connections/{id}/rotate-token",
		"POST /api/scim-connections/{id}/revoke-current-token",
		"POST /api/scim-connections/{id}/revoke-previous-token",
	}

	staticRules = map[string][]string{
		types.GroupAdmin:         adminAndOwnerRules,
		types.GroupAdminReadonly: adminReadonlyRules,
		types.GroupOwner:         append(slices.Clone(adminAndOwnerRules), ownerRules...),
		types.GroupAuditor: {
			"GET /api/llm-gateway/",
			"GET /api/model-proxy",
			"GET /api/model-proxy/usage",
			"GET /api/admin-api-keys",
			"GET /api/admin-api-keys/{id}",
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/",
			"GET /api/egress-audit-logs",
			"GET /api/egress-audit-logs/",
			"GET /api/llm-audit-logs",
			"GET /api/llm-audit-logs/",
			"GET /api/llm-audit-logs/filter-options/",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/",
			"GET /api/mcp-capacity",
			"GET /api/users",
			"GET /api/users/",
			"GET /api/groups",
			"GET /api/groups/",
			"GET /api/group-role-assignments",
			"GET /api/group-role-assignments/",
			"GET /api/mcp-catalogs/",
			"GET /api/system-mcp-catalogs",
			"GET /api/system-mcp-catalogs/",
			"GET /api/mcp-webhook-validations",
			"GET /api/mcp-webhook-validations/",
			"GET /api/mcp-servers/",
			"GET /api/model-access-policies",
			"GET /api/model-access-policies/",
			"GET /api/user-default-role-settings",
			"GET /api/k8s-settings",
			"GET /api/image-pull-secrets/capability",
			"GET /api/image-pull-secrets",
			"GET /api/image-pull-secrets/",
			"GET /api/git-credentials",
			"GET /api/git-credentials/",
			"POST /api/auth-providers/{id}/reveal",
			"GET /api/scim-connections",
			"GET /api/scim-connections/",
			"GET /api/local-auth/users",
			"GET /api/local-auth/users/",
			"GET /api/workspaces/",
			"GET /api/vmcps",
			"GET /api/vmcps/",
			"GET /api/vmcp-instances",
			"GET /api/vmcp-instances/",
			"/api/audit-log-exports",
			"/api/audit-log-exports/",
			"/api/scheduled-audit-log-exports",
			"/api/scheduled-audit-log-exports/",
			"/api/storage-credentials",
			"/api/storage-credentials/",
			"GET /api/skill-repositories",
			"GET /api/skill-repositories/",
			"GET /api/skill-access-rules",
			"GET /api/skill-access-rules/",
			"GET /api/enforcement-decisions",
			"GET /api/enforcement-decisions/",
			"GET /api/devices/scan-stats",
			"GET /api/devices/mcp-servers/",
			"GET /api/devices/skills",
			"GET /api/devices/skills/",
			"GET /api/devices/clients",
			"GET /api/devices/clients/",
			"GET /api/mdm/configurations",
			"GET /api/mdm/configurations/",
			"GET /api/mdm/devices",
			"GET /api/mdm/asset-source",
			"GET /api/mdm/assets",
			"GET /api/token-usage",
			"GET /api/total-token-usage",
			"GET /api/token-usage-stats",
		},
		anyGroup: {
			"POST /api/claude-runners/spawn",
			"POST /api/claude-runners/{instance}/credentials",
			"POST /api/claude-runners/{instance}/finished",
			"POST /api/claude-runners/{instance}/status",
			// Allow access to the oauth2 endpoints
			"/oauth2/",

			"GET /api/token-request/{id}",
			"POST /api/token-request",
			"GET /api/token-request/{id}/{namespace}/{name}",

			"GET /api/oauth/start/{id}/{namespace}/{name}",

			"GET /api/bootstrap",
			"POST /api/bootstrap/login",
			"POST /api/bootstrap/logout",
			"POST /api/local-auth/activate",

			"GET /api/app-oauth/authorize/{id}",
			"GET /api/app-oauth/refresh/{id}",
			"GET /api/app-oauth/callback/{id}",
			"GET /api/app-oauth/get-token/{id}",
			"GET /api/app-oauth/get-token",

			"GET /api/healthz",

			"GET /api/app-preferences",

			"GET /api/auth-providers",
			"GET /api/auth-providers/{id}",

			"GET  /.well-known/",
			"POST /oauth/register/{mcp_id}",
			"POST /oauth/register",
			"GET  /oauth/authorize/{mcp_id}",
			"GET  /oauth/authorize",
			"POST /oauth/token/{mcp_id}",
			"POST /oauth/token",
			"GET  /oauth/jwks.json",
			"GET  /oauth/client-metadata.json",
			// Pending UI login state is a short-lived, opaque capability.
			"GET  /oauth/mcp/login/{state}",

			// Allow any user to read stored images.
			// This allows the UI to display custom images to unauthenticated users.
			"GET /api/image/{image_id}",

			// API Key authentication webhook (called by nanobot shim)
			// This endpoint validates the API key passed in the header
			"POST /api/api-keys/auth",
		},

		types.GroupBasic: {
			"GET /v0.1",
			"GET /v0.1/",
		},

		types.GroupMCP: {
			"GET /v0.1",
			"GET /v0.1/",
		},

		types.GroupAPI: {
			"GET /api/claude-sessions",
			"GET /api/claude-sessions/{session_id}",
			"DELETE /api/claude-sessions/{session_id}",
			"GET /api/claude-environments",
			"PUT /api/claude-environments/{environment}/agent",
			"DELETE /api/claude-environments/{environment}/agent",
			"POST /api/token-request/verify",
			// Whether Claude subscriptions may be used, for the gateway's setup instructions.
			"GET /api/llm-gateway/settings",
			"POST /api/local-agent-audit-logs",
			"GET /api/models",
			"GET /api/model-providers",
			"GET /api/users",
			"GET /api/groups",

			"GET /api/app-notification",

			// Allow authenticated users to read servers and entries from MCP catalogs.
			// Filtering is handled in the handler.
			"GET /api/all-mcps/entries",
			"GET /api/all-mcps/servers",

			// Audit log access for own servers (filtered in handler)
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/detail/{audit_log_id}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",

			// API key management for user's own keys
			"POST /api/api-keys",
			"GET /api/api-keys",
			"GET /api/api-keys/{id}",
			"DELETE /api/api-keys/{id}",

			"GET /api/users",
			"GET /api/users/{user_id}",
			"GET /api/default-model-aliases",
			"/api/oauth/redirect/{namespace}/{name}",
			"DELETE /api/me",
			"PATCH /api/me",
			"POST /api/logout-all",
			"GET /api/version",
			"GET /api/default-k8s-settings",
			"GET /api/license",
			"GET /api/setup/oauth-complete",

			// Users should be able to get the connected tunnel information
			// so they can see which MCP servers that use the tunnel are available.
			"GET /api/tunnels",
		},

		types.GroupPowerUserPlus: {
			"GET /api/groups",
		},

		types.GroupPowerUser: {
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",
		},

		types.GroupAuthenticated: {
			"GET /oauth/userinfo",
			"GET /api/me",
			"POST /api/local-auth/change-password",
		},

		types.GroupSkills: {
			// Skill discovery and download are filtered in the handler.
			"GET /api/skills",
		},

		types.GroupLLM: {
			"/api/llm-proxy/",
		},

		types.GroupDeviceScans: {
			// Device scans: any authenticated user can submit a scan via
			// `obot scan` and read the scans they themselves submitted.
			// Clamp list results to SubmittedBy == req.User.GetUID()
			"POST /api/devices/scans",
			"GET /api/devices/scans",

			// Credentials that can submit scans can also submit local agent tool call audit logs.
			"POST /api/local-agent-audit-logs",

			// Devices ask for a synchronous enforcement decision before running a tool call.
			"POST /api/enforcement/decisions",
		},

		types.GroupDeviceEnroll: {
			// A device enrollment token authenticates as its configuration and may
			// only enroll a device — nothing else.
			"POST /api/mdm/enroll",
		},

		MetricsGroup: {
			"/debug/metrics",
			"GET /api/llm-gateway/metrics",
		},
	}

	devModeRules = map[string][]string{
		anyGroup: {
			"/node_modules/",
			"/@fs/",
			"/.svelte-kit/",
			"/@vite/",
			"/@id/",
			"/src/",
		},
	}
)

// RegistrySettingsReader supplies shared, request-time anonymous discovery policy.
type RegistrySettingsReader interface {
	AnonymousRegistryAccessEnabled(context.Context) (bool, error)
}

type Authorizer struct {
	registrySettings RegistrySettingsReader
	rules            []rule
	cache            kclient.Client
	uncached         kclient.Client
	gatewayClient    *client.Client
	apiResources     map[string]*pathMatcher
	acrHelper        *accesscontrolrule.Helper
	skillHelper      *skillaccessrule.Helper
}

type rule struct {
	group string
	mux   *http.ServeMux
}

// fake is a fake handler that does fake things
type fake struct{}

func NewAuthorizer(gatewayClient *client.Client, cache, uncached kclient.Client, devMode bool, acrHelper *accesscontrolrule.Helper, skillHelper *skillaccessrule.Helper, registrySettings RegistrySettingsReader) *Authorizer {
	apiBasedResources := make(map[string]*pathMatcher, len(apiResources))
	for group, resources := range apiResources {
		apiBasedResources[group] = newPathMatcher(resources...)
	}

	return &Authorizer{
		rules:            defaultRules(devMode),
		registrySettings: registrySettings,
		cache:            cache,
		uncached:         uncached,
		gatewayClient:    gatewayClient,
		apiResources:     apiBasedResources,
		acrHelper:        acrHelper,
		skillHelper:      skillHelper,
	}
}

// IsMCPRegistryRequest reports whether a request targets the MCP Registry API.
func IsMCPRegistryRequest(req *http.Request) bool {
	return req.URL.Path == "/v0.1" || strings.HasPrefix(req.URL.Path, "/v0.1/")
}

func (a *Authorizer) Authorize(req *http.Request, userInfo user.Info) bool {
	// The SCIM endpoint serves only a SCIM connection's principal, and that principal nothing else. There is at most
	// one connection, so the path does not name it.
	isSCIM := req.URL.Path == strings.TrimSuffix(scimPathPrefix, "/") || strings.HasPrefix(req.URL.Path, scimPathPrefix)
	if slices.Contains(userInfo.GetGroups(), types.GroupSCIM) {
		return isSCIM && userInfo.GetUID() != ""
	}

	if isSCIM {
		return false
	}

	// Tunnel credentials are deliberately non-user principals. Keep this check
	// ahead of anyGroup and UI authorization so the credential cannot inherit
	// baseline routes intended for ordinary users.
	if slices.Contains(userInfo.GetGroups(), types.GroupTunnel) {
		if req.Method != http.MethodGet {
			return false
		}
		_, ok := tunnelResources.Match(req)
		return ok
	}
	// Peer credentials are internal, non-user principals used only to connect
	// remotedialer servers running on different Obot replicas.
	if slices.Contains(userInfo.GetGroups(), types.GroupTunnelPeer) {
		if req.Method != http.MethodGet {
			return false
		}
		_, ok := tunnelPeerResources.Match(req)
		return ok
	}
	// Bridge credentials are likewise internal, non-user principals. They may
	// use any HTTP method, but only against one exact encoded bridge target.
	if slices.Contains(userInfo.GetGroups(), types.GroupTunnelBridge) {
		_, ok := tunnelBridgeResources.Match(req)
		return ok
	}

	// Internal principals have already been restricted above. Anonymous grants
	// apply only to registry discovery, and never override authenticated visibility.
	if slices.Contains(userInfo.GetGroups(), UnauthenticatedGroup) &&
		(req.Method == http.MethodGet || req.Method == http.MethodHead) && IsMCPRegistryRequest(req) &&
		a.registrySettings != nil {
		enabled, err := a.registrySettings.AnonymousRegistryAccessEnabled(req.Context())
		if err != nil {
			slog.Error("Denied anonymous registry discovery because settings could not be read", "error", err)
			return false
		}
		return enabled
	}

	user := newUser(userInfo)
	for _, r := range a.rules {
		if r.group == anyGroup || slices.Contains(user.GetGroups(), r.group) {
			if _, pattern := r.mux.Handler(req); pattern != "" {
				return true
			}
		}
	}

	return a.authorizeAPIResources(req, user) || a.checkOAuthClient(req) || a.checkUI(req, user)
}

func (a *Authorizer) get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object) error {
	err := a.cache.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) {
		err = a.uncached.Get(ctx, key, obj)
	}
	return err
}

func defaultRules(devMode bool) []rule {
	f := (*fake)(nil)
	rules := rulesFromStatic(staticRules)

	if devMode {
		for group := range devModeRules {
			rule := rule{
				group: group,
				mux:   http.NewServeMux(),
			}
			for _, url := range devModeRules[group] {
				rule.mux.Handle(url, f)
			}
			rules = append(rules, rule)
		}
	}

	return rules
}

func rulesFromStatic(static map[string][]string) []rule {
	var (
		f     = (*fake)(nil)
		rules []rule
	)
	for group := range static {
		rule := rule{
			group: group,
			mux:   http.NewServeMux(),
		}
		seen := map[string]struct{}{}
		for _, url := range static[group] {
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			rule.mux.Handle(url, f)
		}
		rules = append(rules, rule)
	}
	return rules
}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
