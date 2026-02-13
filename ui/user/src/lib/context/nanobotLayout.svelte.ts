import { getContext, hasContext, setContext } from 'svelte';

export const LAYOUT_CONTEXT = 'nanobot-layout';

export interface Layout {
	sidebarOpen?: boolean;
	quickBarAccessOpen?: boolean;
}

export function initLayout() {
	if (hasContext(LAYOUT_CONTEXT)) {
		return;
	}
	const data = $state<Layout>({
		sidebarOpen: false,
		quickBarAccessOpen: false
	});
	setContext(LAYOUT_CONTEXT, data);
}

export function getLayout(): Layout {
	if (!hasContext(LAYOUT_CONTEXT)) {
		throw new Error('layout context not initialized');
	}
	return getContext<Layout>(LAYOUT_CONTEXT);
}
