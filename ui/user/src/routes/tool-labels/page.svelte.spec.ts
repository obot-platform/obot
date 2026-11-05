import { page as appPage } from '$app/state';
import { preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ToolLabelsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderPage() {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(ToolLabelsPage);
}
afterEach(() => {
	for (const key of ['resourceType', 'resourceID']) (appPage.url as URL).searchParams.delete(key);
});
it('opens the central overview without a resource query', async () => {
	worker.use(
		http.get('/api/provenance/label-inventory', () =>
			HttpResponse.json({
				resources: [],
				observedTools: [],
				capabilities: ['reads-private'],
				contentLabels: [],
				contentTagging: { enabled: false },
				supervisorEnabled: true
			})
		)
	);
	await renderPage();
	await expect.element(page.getByRole('tab', { name: 'Resources' })).toBeVisible();
	await expect
		.element(page.getByText(/No resources with saved tools or label records/))
		.toBeVisible();
});
it('retains the resource-specific review page when a resource is selected', async () => {
	(appPage.url as URL).searchParams.set('resourceType', 'vmcp');
	(appPage.url as URL).searchParams.set('resourceID', 'clinical-vmcp');
	worker.use(
		http.get('/api/provenance/tool-label-suggestions', () =>
			HttpResponse.json({
				resourceType: 'vmcp',
				resourceID: 'clinical-vmcp',
				displayName: 'Clinical vMCP',
				items: []
			})
		)
	);
	await renderPage();
	await expect.element(page.getByRole('heading', { name: 'Clinical vMCP' })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'Suggest labels' })).toBeDisabled();
	await expect.element(page.getByRole('tab', { name: 'Resources' })).not.toBeInTheDocument();
});
