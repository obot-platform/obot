import { page as pageState } from '$app/state';

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
	pageState.url = next as unknown as URL & { pathname: `/${string}` | `/${string}/${string}` };
	return Promise.resolve();
}
