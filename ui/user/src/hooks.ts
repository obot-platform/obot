import type { Reroute } from '@sveltejs/kit';

const ADMIN_MCP_SERVERS_PREFIX = '/admin/mcp-catalog';
const ADMIN_MCP_DEPLOYMENTS_PREFIX = '/admin/mcp-deployments';
const ADMIN_AGENT_AUTH_SCOPES_PREFIX = '/admin/agent-auth-scopes';
const ADMIN_SKILLS_PREFIX = '/admin/skills';
const ADMIN_DASHBOARD_PREFIX = '/admin/dashboard';
const ADMIN_DEVICES_PREFIX = '/admin/devices';
const ADMIN_MCP_AUDIT_LOGS_PREFIX = '/admin/audit-logs/exports';
const ADMIN_LLM_AUDIT_LOGS_PREFIX = '/admin/llm-audit-logs/exports';
const ADMIN_HOSTED_AGENTS_PREFIX = '/admin/hosted-agents';

export const reroute: Reroute = ({ url }) => {
	const { pathname } = url;

	if (pathname.startsWith(`${ADMIN_MCP_DEPLOYMENTS_PREFIX}/`)) {
		return pathname.replace(ADMIN_MCP_DEPLOYMENTS_PREFIX, '/mcp-catalog');
	}

	if (
		pathname === ADMIN_MCP_SERVERS_PREFIX ||
		pathname.startsWith(`${ADMIN_MCP_SERVERS_PREFIX}/`)
	) {
		return pathname.replace(ADMIN_MCP_SERVERS_PREFIX, '/mcp-catalog');
	}

	if (
		pathname === ADMIN_AGENT_AUTH_SCOPES_PREFIX ||
		pathname.startsWith(`${ADMIN_AGENT_AUTH_SCOPES_PREFIX}/`)
	) {
		return pathname.replace(ADMIN_AGENT_AUTH_SCOPES_PREFIX, '/agent-auth-scopes');
	}

	if (pathname === ADMIN_SKILLS_PREFIX || pathname.startsWith(`${ADMIN_SKILLS_PREFIX}/`)) {
		return pathname.replace(ADMIN_SKILLS_PREFIX, '/skills');
	}

	if (pathname === ADMIN_DASHBOARD_PREFIX) {
		return pathname.replace(ADMIN_DASHBOARD_PREFIX, '/dashboard');
	}

	if (pathname === ADMIN_DEVICES_PREFIX || pathname.startsWith(`${ADMIN_DEVICES_PREFIX}/`)) {
		// support devices/[id] routes to /inventory/devices/[id]
		const suffix = pathname.slice(ADMIN_DEVICES_PREFIX.length);
		if (!suffix || suffix === '/') return '/inventory';
		if (/^\/(?:skills|mcp-servers|clients)(?:\/|$)/.test(suffix)) return `/inventory${suffix}`;
		return `/inventory/devices${suffix}`;
	}

	if (pathname.startsWith(ADMIN_MCP_AUDIT_LOGS_PREFIX)) {
		return pathname.replace(ADMIN_MCP_AUDIT_LOGS_PREFIX, '/audit-logs/mcp/exports');
	}

	if (pathname.startsWith(ADMIN_LLM_AUDIT_LOGS_PREFIX)) {
		return pathname.replace(ADMIN_LLM_AUDIT_LOGS_PREFIX, '/audit-logs/llm/exports');
	}

	if (pathname === ADMIN_HOSTED_AGENTS_PREFIX || pathname.startsWith(ADMIN_HOSTED_AGENTS_PREFIX)) {
		return pathname.replace(ADMIN_HOSTED_AGENTS_PREFIX, '/hosted-agents');
	}
};
