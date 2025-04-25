import type { ProjectMCP } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('mcps');

export interface ProjectMCPContext {
	items: ProjectMCP[];
}

export function getProjectMCPs() {
	if (!hasContext(Key)) {
		throw new Error('Project MCPs not initialized');
	}
	return getContext<ProjectMCPContext>(Key);
}

export function initProjectMCPs(mcps: ProjectMCP[]) {
	const data = $state<ProjectMCPContext>({ items: mcps });
	setContext(Key, data);
}
