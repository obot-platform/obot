import { ChatService, type ProjectMCP } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('mcps');

export type ProjectMcpItem = ProjectMCP & {
	oauthURL?: string;
	authenticated?: boolean;
};

export interface ProjectMCPContext {
	items: ProjectMcpItem[];
}

export function getProjectMCPs() {
	if (!hasContext(Key)) {
		throw new Error('Project MCPs not initialized');
	}
	return getContext<ProjectMCPContext>(Key);
}

export async function validateOauthProjectMcps(projectMcps: ProjectMcpItem[]) {
	const updatingMcps = [...projectMcps];
	let needsMcpOauth = false;
	for (let i = 0; i < updatingMcps.length; i++) {
		if (updatingMcps[i].authenticated) {
			continue;
		}

		const mcp = updatingMcps[i];
		const oauthURL = await ChatService.getMcpServerOauthURL(mcp.id);
		if (oauthURL) {
			updatingMcps[i].oauthURL = oauthURL;
			needsMcpOauth = true;
		} else {
			updatingMcps[i].authenticated = true; // does not require oauth, so we can assume it's authenticated
		}
	}
	if (needsMcpOauth) {
		return updatingMcps;
	}

	return [];
}

export function initProjectMCPs(mcps: ProjectMcpItem[]) {
	const data = $state<ProjectMCPContext>({ items: mcps });
	setContext(Key, data);
}
