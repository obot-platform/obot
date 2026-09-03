import type { Reroute } from '@sveltejs/kit';

const ADMIN_MCP_SERVERS_PREFIX = '/admin/mcp-catalog';
const ADMIN_MCP_DEPLOYMENTS_PREFIX = '/admin/mcp-deployments';
const ADMIN_AGENT_AUTH_SCOPES_PREFIX = '/admin/agent-auth-scopes';
const ADMIN_SKILLS_PREFIX = '/admin/skills';
const ADMIN_DASHBOARD_PREFIX = '/admin/dashboard';
const ADMIN_DEVICES_PREFIX = '/admin/devices';

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
		return pathname.replace(ADMIN_DEVICES_PREFIX, '/inventory');
	}
};
