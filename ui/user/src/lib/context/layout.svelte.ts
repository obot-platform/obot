import { getContext, hasContext, setContext } from 'svelte';

export interface Layout {
	threadsOpen?: boolean;
	projectEditorOpen?: boolean;
	fileEditorOpen?: boolean;
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

export function hasLayout(): boolean {
	return hasContext('layout');
}
