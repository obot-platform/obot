import { page as pageState } from '../../../node_modules/@sveltejs/kit/src/runtime/client/state.svelte.js';

export function applyTestGoto(url: string | URL): Promise<void> {
	const value = String(url);
	const next = new URL(window.location.href);
	try {
		const parsed = new URL(value, next.origin);
		next.pathname = parsed.pathname;
		next.search = parsed.search;
		next.hash = parsed.hash;
	} catch {
		const queryIndex = value.indexOf('?');
		next.search = queryIndex >= 0 ? value.slice(queryIndex) : '';
	}
	pageState.url = next;
	return Promise.resolve();
}
