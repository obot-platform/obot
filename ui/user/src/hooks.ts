import type { Reroute } from '@sveltejs/kit';

const ADMIN_MCP_SERVERS_PREFIX = '/admin/mcp-catalog';
const ADMIN_MCP_DEPLOYMENTS_PREFIX = '/admin/mcp-deployments';
const ADMIN_AGENT_AUTH_SCOPES_PREFIX = '/admin/agent-auth-scopes';

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
};
