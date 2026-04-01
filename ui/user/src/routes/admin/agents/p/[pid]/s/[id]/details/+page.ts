import { handleRouteError } from '$lib/errors';
import { AdminService, NanobotService, type MCPCatalogServer } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { pid, id } = params;
	const { profile } = await parent();
	const mcpServerId = `ms1${id}`;
	let mcpServer: MCPCatalogServer | undefined;
	let agent: ProjectV2Agent | undefined;

	try {
		agent = await NanobotService.getProjectV2Agent(pid, id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/admin/agents/p/${pid}/s/${id}/details`, profile);
	}

	try {
		mcpServer = await AdminService.getMCPServerById(mcpServerId, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/admin/agents/p/${pid}/s/${id}/details`, profile);
	}

	return {
		mcpServer,
		agent,
		id: mcpServerId
	};
};
