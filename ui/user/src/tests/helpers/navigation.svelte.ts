import { SvelteURL } from 'svelte/reactivity';

type AppState = typeof import('$app/state');

class TestPage {
	data = $state.raw<AppState['page']['data']>({} as AppState['page']['data']);
	form = $state.raw<AppState['page']['form']>(null);
	error = $state.raw<AppState['page']['error']>(null);
	params = $state.raw<AppState['page']['params']>({} as AppState['page']['params']);
	route = $state.raw<AppState['page']['route']>({ id: null });
	state = $state.raw<AppState['page']['state']>({});
	status = $state.raw<AppState['page']['status']>(200);
	url = $state.raw(new SvelteURL('http://localhost/'));
}

const testPage = new TestPage();

function currentUrl() {
	const current = testPage.url;
	if (current.protocol === 'http:' || current.protocol === 'https:') {
		return new SvelteURL(current.href);
	}
	return new SvelteURL(window.location.href);
}

export function createAppState(actual: AppState) {
	testPage.data = actual.page.data;
	testPage.form = actual.page.form;
	testPage.error = actual.page.error;
	testPage.params = actual.page.params;
	testPage.route = actual.page.route;
	testPage.state = actual.page.state;
	testPage.status = actual.page.status;
	testPage.url = new SvelteURL(actual.page.url.href);
	return {
		page: testPage,
		navigating: actual.navigating,
		updated: actual.updated
	};
}

export function applyTestGoto(url: string | URL): Promise<void> {
	const value = String(url);
	const next = currentUrl();
	try {
		const parsed = new SvelteURL(value, next.origin);
		next.pathname = parsed.pathname;
		next.search = parsed.search;
		next.hash = parsed.hash;
	} catch {
		const queryIndex = value.indexOf('?');
		next.search = queryIndex >= 0 ? value.slice(queryIndex) : '';
	}
	testPage.url = next;
	return Promise.resolve();
}
