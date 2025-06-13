import { getContext, hasContext, setContext } from 'svelte';

export const LAYOUT_CONTEXT = 'layout';

export interface Layout {
	sidebarOpen?: boolean;
}

export function initLayout() {
	const data = $state<Layout>({
		sidebarOpen: true
	});
	setContext(LAYOUT_CONTEXT, data);
}

export function getLayout(): Layout {
	if (!hasContext(LAYOUT_CONTEXT)) {
		throw new Error('layout context not initialized');
	}
	return getContext<Layout>(LAYOUT_CONTEXT);
}
