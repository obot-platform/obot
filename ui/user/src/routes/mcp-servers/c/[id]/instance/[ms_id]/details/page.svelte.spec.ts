import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
import { preparePageData } from '../../../../../../../tests/helpers/pageData';
import {
	createMcpServerDetailsFixtures,
	getK8sServerDetailResponse,
	getServerK8sSettingsResponse
} from '../../../../../../../tests/mocks/data';
import { worker } from '../../../../../../../tests/mocks/worker';
import type { PageData } from './$types';
import DetailsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

function mockCatalogServerApis(server: MCPCatalogServer) {
	worker.use(
		http.get(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}`, () =>
			HttpResponse.json(server)
		),
		http.get(`/api/mcp-servers/${server.id}`, () => HttpResponse.json(server)),
		http.get(`/api/users/${server.userID}`, () => HttpResponse.json(fixtures.associatedUser))
	);
}

function mockHostedK8sApis(server: MCPCatalogServer, catalogEntry: MCPCatalogEntry) {
	worker.use(
		http.get(`/api/mcp-servers/${server.id}/details`, () =>
			HttpResponse.json(getK8sServerDetailResponse)
		),
		http.get(
			`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/entries/${catalogEntry.id}/servers/${server.id}/k8s-settings-status`,
			() => HttpResponse.json(getServerK8sSettingsResponse)
		),
		http.post(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${server.id}/reveal`, () =>
			HttpResponse.json({})
		)
	);
}

async function renderInstanceDetailsPage(catalogEntry: MCPCatalogEntry, server: MCPCatalogServer) {
	mockCatalogServerApis(server);
	const data = await preparePageData<PageData>({
		catalogEntry,
		mcpServerId: server.id
	});
	return render(DetailsPage, { data });
}

describe('MCP Catalog entry instance details page (admin)', () => {
	it('hosted singleUser server shows k8s details and Associated User table', async () => {
		mockHostedK8sApis(fixtures.serverSingle, fixtures.entrySingle);
		await renderInstanceDetailsPage(fixtures.entrySingle, fixtures.serverSingle);

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
			.element(page.getByRole('heading', { name: 'Associated User', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(fixtures.associatedUser.email, { exact: true }).first())
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Connected Users', exact: true }))
			.not.toBeInTheDocument();
	});

	it('remote server shows Associated User table and OAuth metadata', async () => {
		await renderInstanceDetailsPage(fixtures.entryRemote, fixtures.serverRemote);

		await expect
			.element(page.getByRole('heading', { name: 'Associated User', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(fixtures.associatedUser.email, { exact: true }).first())
			.toBeVisible();

		await expect
			.element(page.getByRole('heading', { name: 'OAuth Metadata', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('Discovered', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Protected Resource URL', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText('https://example.com/.well-known/oauth-protected-resource', { exact: true })
			)
			.toBeVisible();

		await expect.element(page.getByText('Deployment', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Recent Events', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Deployment Logs', exact: true }))
			.not.toBeInTheDocument();
	});

	it('composite server shows Connected Users table and MCP Servers links to children', async () => {
		worker.use(
			http.get(`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/entries/all-servers`, () =>
				HttpResponse.json({ items: [fixtures.serverCompositeChild] })
			),
			http.get('/api/workspaces/all-entries/all-servers', () => HttpResponse.json({ items: [] }))
		);
		await renderInstanceDetailsPage(fixtures.entryComposite, fixtures.serverComposite);

		await expect
			.element(page.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText('Composite Child Entry', { exact: true }).first())
			.toBeVisible();
		await expect
			.element(page.getByText(`(${fixtures.serverCompositeChild.id})`, { exact: true }))
			.toBeVisible();

		await expect
			.element(page.getByRole('heading', { name: 'Connected Users', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(fixtures.associatedUser.email, { exact: true }).first())
			.toBeVisible();

		await expect.element(page.getByText('Deployment', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Associated User', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'OAuth Metadata', exact: true }))
			.not.toBeInTheDocument();
	});
});
