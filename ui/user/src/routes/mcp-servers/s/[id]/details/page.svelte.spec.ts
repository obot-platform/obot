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
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

// Called each time the page asks for the server's k8s settings status. The page shows its details twice, once
// before it loads the server's instances and users and again after, and each time asks for the status, which nothing
// visible waits for. So every test waits for both requests before it ends: one made after the test's handlers are
// reset is unhandled and fails the whole run.
let settingsLoaded = vi.fn();

afterEach(async () => {
	await vi.waitFor(() => expect(settingsLoaded).toHaveBeenCalledTimes(2));
});

function mockMultiUserDetailsApis() {
	const server = fixtures.serverMulti;
	settingsLoaded = vi.fn();

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
			() => {
				settingsLoaded();
				return HttpResponse.json(getServerK8sSettingsResponse);
			}
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
			.element(page.getByText('View Audit Logs', { exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Associated User', exact: true }))
			.not.toBeInTheDocument();
	});
});

describe('owners of the instances', () => {
	it('names a user, an agent and a deleted agent, and looks up no user for an agent', async () => {
		const lookedUp = vi.fn();
		mockMultiUserDetailsApis();
		worker.use(
			http.get(
				`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${fixtures.serverMulti.id}/instances`,
				() =>
					HttpResponse.json({
						items: [
							fixtures.multiUserInstance,
							{ ...fixtures.multiUserInstance, id: 'instance-agent', userID: 'agent:7' },
							{ ...fixtures.multiUserInstance, id: 'instance-gone', userID: 'agent:9' }
						]
					})
			),
			// Only agents are listed for names, and nothing asks the user service about an agent.
			http.get('/api/agents', () =>
				HttpResponse.json({
					items: [
						{ id: '7', displayName: 'Build Bot', status: 'active', activeAPIKeyCount: 1 },
						{ id: '9', displayName: 'Old Bot', status: 'deleted', activeAPIKeyCount: 0 }
					]
				})
			),
			http.get('/api/users/:id', ({ params }) => {
				lookedUp(params.id);
				return HttpResponse.json(fixtures.associatedUser);
			})
		);
		const data = await preparePageData<PageData>({
			mcpServer: fixtures.serverMulti,
			id: fixtures.serverMulti.id
		});
		render(DetailsPage, { data });

		await expect
			.element(page.getByText(fixtures.associatedUser.email, { exact: true }).first())
			.toBeVisible();
		await expect.element(page.getByText('Build Bot', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Old Bot (Deleted)', { exact: true })).toBeVisible();
		// The user has no badge, and each agent row has one.
		await expect.element(page.getByText('Agent', { exact: true }).nth(1)).toBeVisible();
		await expect.element(page.getByText('Agent', { exact: true }).nth(2)).not.toBeInTheDocument();
		await expect.element(page.getByText('agent:7')).not.toBeInTheDocument();
		expect(lookedUp).not.toHaveBeenCalled();
	});

	it('shows an agent by its ID when agents cannot be listed', async () => {
		mockMultiUserDetailsApis();
		worker.use(
			http.get(
				`/api/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${fixtures.serverMulti.id}/instances`,
				() =>
					HttpResponse.json({
						items: [{ ...fixtures.multiUserInstance, id: 'instance-agent', userID: 'agent:7' }]
					})
			),
			http.get('/api/agents', () => HttpResponse.json({ error: 'forbidden' }, { status: 403 }))
		);
		const data = await preparePageData<PageData>({
			mcpServer: fixtures.serverMulti,
			id: fixtures.serverMulti.id
		});
		render(DetailsPage, { data });

		await expect.element(page.getByText('Agent 7', { exact: true })).toBeVisible();
	});
});
