import { PROJECT_MCP_SERVER_NAME } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { ChatService, EditorService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const projects = await ChatService.listProjects({ fetch });
		let mcpServersProject = projects.items.find((p) => p.name === PROJECT_MCP_SERVER_NAME);
		if (!mcpServersProject) {
			const response = await EditorService.createObot({ fetch, name: PROJECT_MCP_SERVER_NAME });
			mcpServersProject = response;
		}

		return {
			project: mcpServersProject
		};
	} catch (e) {
		handleRouteError(e, `/mcp-servers`, profile.current);
	}
};
