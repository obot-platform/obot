import type { AssistantTool } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

const PROJECT_TOOLS_CONTEXT_NAME = 'project-tools';
export interface ProjectTools {
	tools: AssistantTool[];
	maxTools: number;
}

export function getProjectTools(): ProjectTools {
	if (!hasContext(PROJECT_TOOLS_CONTEXT_NAME)) {
		throw new Error('layout context not initialized');
	}
	return getContext<ProjectTools>(PROJECT_TOOLS_CONTEXT_NAME);
}

export function initProjectTools(init: ProjectTools) {
	const projectTools = $state<ProjectTools>(init);
	setContext(PROJECT_TOOLS_CONTEXT_NAME, projectTools);
}
