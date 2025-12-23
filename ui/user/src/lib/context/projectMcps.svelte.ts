import { ChatService, type ProjectMCP } from '$lib/services';
import { errors } from '$lib/stores';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('mcps');

export type ProjectMcpItem = ProjectMCP & {
	oauthURL?: string;
	authenticated?: boolean;
	configured?: boolean;
	needsURL?: boolean;
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

export async function validateOauthProjectMcps(
	assistantID: string,
	projectID: string,
	projectMcps: ProjectMcpItem[],
	all: boolean = false
) {
	const updatingMcps = [...projectMcps];
	let needsMcpOauth = false;
	for (let i = 0; i < updatingMcps.length; i++) {
		if (updatingMcps[i].authenticated) {
			continue;
		}

		try {
			const mcp = updatingMcps[i];
			const oauthURL = await ChatService.getProjectMcpServerOauthURL(
				assistantID,
				projectID,
				mcp.id!,
				{ dontLogErrors: true }
			);
			if (oauthURL) {
				updatingMcps[i].oauthURL = oauthURL;
				needsMcpOauth = true;
			} else {
				updatingMcps[i].authenticated = true; // does not require oauth, so we can assume it's authenticated
			}
		} catch (err) {
			// Skip 400 errors related to missing AUTHORIZATION config, This will be handled in the UI by showing error indicators and requesting Auth Token configuration.
			if (
				err instanceof Error &&
				err.message.includes('400') &&
				err.message.includes('AUTHORIZATION')
			) {
				continue;
			}

			if (err instanceof Error) {
				errors.items.push(err);
			}
		}
	}
	if (needsMcpOauth || all) {
		return updatingMcps;
	}

	return [];
}

export function initProjectMCPs(mcps: ProjectMcpItem[]) {
	const data = $state<ProjectMCPContext>({ items: mcps });
	setContext(Key, data);
}
