import type { Reroute } from '@sveltejs/kit';

const ADMIN_DEVICES_PREFIX = '/admin/devices';
const ADMIN_MCP_SERVERS_PREFIX = '/admin/mcp-catalog';

export const reroute: Reroute = ({ url }) => {
	const { pathname } = url;

	if (pathname === ADMIN_DEVICES_PREFIX || pathname.startsWith(`${ADMIN_DEVICES_PREFIX}/`)) {
		return pathname.replace(ADMIN_DEVICES_PREFIX, '/devices');
	}

	if (
		pathname === ADMIN_MCP_SERVERS_PREFIX ||
		pathname.startsWith(`${ADMIN_MCP_SERVERS_PREFIX}/`)
	) {
		return pathname.replace(ADMIN_MCP_SERVERS_PREFIX, '/mcp-catalog');
	}
};
