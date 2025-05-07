import { handleRouteError } from '$lib/errors';
import { ChatService, type MCP, type ProjectTemplate } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let template: ProjectTemplate | undefined;
	let mcps: MCP[] = [];

	try {
		template = await ChatService.getTemplate(params.id, { fetch });
		mcps = await ChatService.listMCPs({ fetch });
		const mcpsMap = new Map(mcps.map((m) => [m.id, m]));
		mcps = (template.mcpServers?.map((id) => mcpsMap.get(id)).filter(Boolean) as MCP[]) || [];
	} catch (e) {
		handleRouteError(e, `/t/${params.id}`, profile.current);
	}

	return {
		template,
		mcps
	};
};
