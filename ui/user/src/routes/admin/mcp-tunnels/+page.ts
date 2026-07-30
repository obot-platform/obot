import { handleRouteError } from '$lib/errors';
import { AdminService, type MCPTunnel, type TunnelConnection } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let mcpTunnels: MCPTunnel[] = [];
	let tunnelConnections: TunnelConnection[] | undefined;

	try {
		mcpTunnels = await AdminService.listMCPTunnels({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/mcp-tunnels', profile.current);
	}

	try {
		tunnelConnections = await AdminService.listTunnelConnections({
			fetch,
			dontLogErrors: true
		});
	} catch {
		tunnelConnections = undefined;
	}

	return {
		mcpTunnels,
		tunnelConnections
	};
};
