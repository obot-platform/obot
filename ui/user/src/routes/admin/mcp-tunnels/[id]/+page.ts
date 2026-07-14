import { handleRouteError } from '$lib/errors';
import { AdminService, type TunnelConnection } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	let mcpTunnel;
	let tunnelConnections: TunnelConnection[] | undefined;

	try {
		mcpTunnel = await AdminService.getMCPTunnel(params.id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/mcp-tunnels/${params.id}`, profile.current);
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
		mcpTunnel,
		tunnelConnections
	};
};
