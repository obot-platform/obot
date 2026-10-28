// Shared spec helpers for the Models page tabs. The tab views get their primary actions from the
// page's nav bar, so specs render them through the page with the tab selected.
import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import type { PageData } from './$types';
import ModelsPage from './+page.svelte';
import { render } from 'vitest-browser-svelte';

export const readyStatus = {
	enabled: true,
	ready: true,
	hasProviders: true,
	modelProviderName: 'llm-gateway-model-provider',
	proxyPath: '/api/llm-proxy'
};

export async function renderModelsTab(view: string, groups: string[] = [Group.ADMIN]) {
	appPage.url.searchParams.set('view', view);
	// Layout's setup splash would otherwise sit on top of the page for admins.
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	const data = await preparePageData<PageData>({
		profile: createMockProfile(groups),
		models: [],
		hasAdminAccess: groups.includes(Group.ADMIN) || groups.includes(Group.AUDITOR)
	});
	return render(ModelsPage, { data });
}

export function resetModelsTab() {
	for (const key of Array.from(appPage.url.searchParams.keys())) {
		appPage.url.searchParams.delete(key);
	}
}
