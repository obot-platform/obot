import type { Project } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

export function initProject(project: Project) {
	const data = $state<Project>(project);
	setContext('project', data);
}

export function getProject(): Project {
	if (!hasContext('project')) {
		throw new Error('project context not initialized');
	}
	return getContext<Project>('project');
}

export function hasProject(): boolean {
	return hasContext('project');
}
