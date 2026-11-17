/**
 * Escape characters that have special meaning in regular expressions.
 */
export function escapeRegExp(value: string): string {
	return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Maps slugs of pages that were removed or renamed between older versions
 * and the latest version to their current equivalents.
 *
 * Keys and values are root-relative paths WITHOUT leading/trailing slashes.
 * An empty string means "fall back to site root".
 *
 * Used by both the postBuild canonical-urls plugin (for static HTML) and
 * the swizzled DocItem/Metadata component (for client-side hydration).
 */
export const PATH_REDIRECTS: Record<string, string> = {
	// Consolidated guides: retained older pages canonicalize to the current guide.
	"configuration/user-roles": "security/policy-coverage",
	"functionality/audit-logs-and-usage": "security/audit-data",
	"functionality/device-management": "device-management/how-sentry-works",
	"functionality/llm-gateway": "llm-gateway/how-it-works",
	"functionality/mcp-access-policies": "mcp-gateway/access",
	"functionality/overview": "security/model",
	"functionality/server-scheduling": "operations/capacity",
	"functionality/skill-access-policies": "registries/publish-skills",
	"functionality/skills": "registries/publish-skills",
	"functionality/user-management": "security/authentication",
	"functionality/virtual-mcps": "mcp-gateway/server-types",
	"installation/cli-setup": "reference/cli-api",
	"installation/enabling-authentication": "configuration/auth-providers",
	"reference/schemas": "reference/catalog-schemas",
	"configuration/encryption-providers/overview": "security/credentials",
	"mcp-gateway/filters-tunnels": "functionality/filters",
	"configuration/mcp-server-oauth-configuration": "mcp-gateway/server-configuration",

	// Pages moved between versions
	"agents/availability": "start-here/choose",
	"agents/first-agent": "start-here/govern",
	"agents/identity": "security/credentials",
	"agents/runtime": "installation/overview",
	"agents/workflows": "registries/publish-skills",
	"concepts/obot-agent": "start-here/choose",
	"functionality/obot-agent-management": "security/model",
	"functionality/workflow-sharing": "registries/publish-skills",
	"functionality/mcp-inspector": "mcp-gateway/inspector",
	architecture: "concepts/architecture",

	// concepts/admin/* — section removed; map to closest equivalents
	"concepts/admin/overview": "security/model",
	"concepts/admin/mcp-servers": "functionality/mcp-servers",
	"concepts/admin/mcp-server-catalogs": "configuration/mcp-server-gitops",
	"concepts/admin/filters": "functionality/filters",
	"concepts/admin/access-control": "security/policy-coverage",

	// concepts/chat/* — section removed; map to current agent concept page
	"concepts/chat/overview": "concepts/obot-agent",
	"concepts/chat/projects": "concepts/obot-agent",
	"concepts/chat/tasks": "concepts/obot-agent",
	"concepts/chat/threads": "concepts/obot-agent",

	// other chat-related pages removed/renamed; map to current agent concept page
	"concepts/obot-chat": "concepts/obot-agent",
	"functionality/chat/overview": "concepts/obot-agent",
	"functionality/chat-management": "concepts/obot-agent",
	"functionality/agent/overview": "concepts/obot-agent",

	// concepts/mcp-gateway sub-pages — restructured into single page
	"concepts/mcp-gateway/overview": "concepts/mcp-gateway",
	"concepts/mcp-gateway/obot-registry": "concepts/mcp-registry",
	"concepts/mcp-gateway/registry-api": "concepts/mcp-registry",
	"concepts/mcp-gateway/servers-and-tools": "concepts/mcp-gateway",

	// functionality renames
	"functionality/api-keys": "functionality/agent-auth-scopes",
	"functionality/message-policies": "functionality/ai-judge-policies",

	// configuration renames
	"configuration/chat-configuration": "configuration/server-configuration",
	"configuration/oauth-configuration":
		"mcp-gateway/server-configuration",

	// Removed sections
	"integrations/ide-client-integration": "",
	"tutorials/github-assistant": "",
	"tutorials/slack-alerts-assistant": "",
	"tutorials/knowledge-assistant": "",
};
