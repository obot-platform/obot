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
	// Pages moved between versions
	architecture: "concepts/architecture",

	// concepts/admin/* — section removed; map to closest equivalents
	"concepts/admin/overview": "functionality/overview",
	"concepts/admin/mcp-servers": "functionality/mcp-servers",
	"concepts/admin/mcp-server-catalogs": "configuration/mcp-server-gitops",
	"concepts/admin/filters": "functionality/filters",
	"concepts/admin/access-control": "configuration/user-roles",

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

	// configuration renames
	"configuration/chat-configuration": "configuration/server-configuration",
	"configuration/oauth-configuration":
		"configuration/mcp-server-oauth-configuration",

	// Removed sections
	"integrations/ide-client-integration": "",
	"tutorials/github-assistant": "",
	"tutorials/slack-alerts-assistant": "",
	"tutorials/knowledge-assistant": "",
};
