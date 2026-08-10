import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { preparePageData } from '../../../../../tests/helpers/pageData';
import {
	createMcpServerDetailsFixtures,
	getK8sServerDetailResponse,
	getServerK8sSettingsResponse,
	listUsersResponse
} from '../../../../../tests/mocks/data';
import { worker } from '../../../../../tests/mocks/worker';
import type { PageData } from './$types';
import DetailsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

function mockMultiUserDetailsApis() {
	const server = fixtures.serverMulti;

	worker.use(
		http.get(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}`, () =>
			HttpResponse.json(server)
		),
		http.get(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}/instances`, () =>
			HttpResponse.json({ items: [fixtures.multiUserInstance] })
		),
		http.get('/api/users', () => HttpResponse.json({ items: listUsersResponse })),
		http.get(`/api/mcp-servers/${server.id}/details`, () =>
			HttpResponse.json(getK8sServerDetailResponse)
		),
		http.get(
			`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}/k8s-settings-status`,
			() => HttpResponse.json(getServerK8sSettingsResponse)
		),
		http.post(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}/reveal`, () =>
			HttpResponse.json({})
		)
	);
}

async function renderDetailsPage() {
	mockMultiUserDetailsApis();
	const data = await preparePageData<PageData>({
		mcpServer: fixtures.serverMulti,
		id: fixtures.serverMulti.id
	});
	return render(DetailsPage, { data });
}

describe('MCP Catalog multi-user server details page (admin)', () => {
	it('hosted multiUser server shows k8s details and Connected Users table', async () => {
		await renderDetailsPage();

		await expect.element(page.getByText('Deployment', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Status', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Healthy', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Recent Events', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Configuration', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Deployment Logs', exact: true }))
			.toBeVisible();

		await expect
			.element(page.getByRole('heading', { name: 'Connected Users', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(fixtures.associatedUser.email, { exact: true }).first())
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Associated User', exact: true }))
			.not.toBeInTheDocument();
	});
});
