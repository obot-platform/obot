import type { Task } from '$lib/services';
import type { EditorItem } from '$lib/services/editor/index.svelte';
import { getContext, hasContext, setContext } from 'svelte';

export interface Layout {
	sidebarOpen?: boolean;
	editTaskID?: string;
	tasks?: Task[];
	items: EditorItem[];
	projectEditorOpen?: boolean;
	fileEditorOpen?: boolean;
	currentThreadID?: string;
}

export function initLayout(layout: Layout) {
	const data = $state<Layout>(layout);
	setContext('layout', data);
}

export function getLayout(): Layout {
	if (!hasContext('layout')) {
		throw new Error('layout context not initialized');
	}
	return getContext<Layout>('layout');
}
