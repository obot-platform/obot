import { page as appPage } from '$app/state';
import { Group, type MCPCatalogEntry } from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { getProfileResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import VMcpsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub'
});

function createIssueTrackerVMcp() {
	return createVMCP({ id: 'vmcp-1', displayName: 'Issue Tracker vMCP' }, [componentEntry]);
}

async function renderPageWithEntries(
	entries: MCPCatalogEntry[],
	creating = false,
	vmcps = [createIssueTrackerVMcp()],
	groups: string[] = [Group.ADMIN]
) {
	if (creating) {
		appPage.url.searchParams.set('new', 'true');
	} else {
		appPage.url.searchParams.delete('new');
	}
	mcpServersAndEntries.current = {
		entries,
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	const data = await preparePageData<PageData>({
		vmcps,
		vmcpRepositories: [],
		gitCredentials: [],
		profile: createMockProfile(groups)
	});
	return render(VMcpsPage, { data });
}

afterEach(() => {
	appPage.url.searchParams.delete('new');
	appPage.url.searchParams.delete('view');
});

describe('vMCPs Page', () => {
	describe('table view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		it('shows vMCPs and Sources tabs', async () => {
			await renderPageWithEntries([componentEntry]);

			await expect.element(page.getByRole('button', { name: 'vMCPs', exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Sources', exact: true }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Create vMCP' })).toBeVisible();
		});

		it('shows the sources empty state', async () => {
			appPage.url.searchParams.set('view', 'sources');
			await renderPageWithEntries([componentEntry]);

			await expect.element(page.getByText('No current Git Source URLs.')).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Add Source URL' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Create vMCP' }))
				.not.toBeInTheDocument();
		});

		it('takes no drops, so it offers no servers to drag', async () => {
			await renderPageWithEntries([componentEntry, slack]);
			await expect
				.element(page.getByRole('button', { name: 'Click to edit Issue Tracker vMCP' }))
				.toBeVisible();

			await expect
				.element(page.getByRole('button', { name: new RegExp('View Slack details') }))
				.not.toBeInTheDocument();
		});

		it('lets admins hide personal vMCPs with the shared filter', async () => {
			const personal = createVMCP({
				id: 'vmcp-personal',
				displayName: 'Personal Notes vMCP',
				userID: getProfileResponse.id
			});
			const shared = createVMCP({
				id: 'vmcp-shared',
				displayName: 'Org Docs vMCP',
				userID: ''
			});
			await renderPageWithEntries([componentEntry], false, [personal, shared]);

			await expect
				.element(page.getByRole('button', { name: 'Click to edit Personal Notes vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Click to edit Org Docs vMCP' }))
				.toBeVisible();

			await page.getByRole('button', { name: 'Filters' }).click();
			await page.getByRole('checkbox', { name: 'Show shared vMCPs only' }).click();
			await tick();

			await expect
				.element(page.getByRole('button', { name: 'Click to edit Org Docs vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Click to edit Personal Notes vMCP' }))
				.not.toBeInTheDocument();
		});

		it('does not offer the shared filter to users without admin access', async () => {
			await renderPageWithEntries(
				[componentEntry],
				false,
				[createIssueTrackerVMcp()],
				[Group.USER]
			);

			await page.getByRole('button', { name: 'Filters' }).click();
			await expect
				.element(page.getByRole('checkbox', { name: 'Show shared vMCPs only' }))
				.not.toBeInTheDocument();
		});
	});

	describe('create view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		function panelCard(name: string) {
			return page.getByRole('button', { name: new RegExp(`View ${name} details`) });
		}

		it('opens the designer canvas when creating a vMCP', async () => {
			await renderPageWithEntries([componentEntry], true, []);

			await expect.element(page.getByRole('button', { name: 'Designer' })).toBeVisible();
			await expect.element(page.getByCSS('[data-vmcp-world]')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Click to edit Issue Tracker vMCP' }))
				.not.toBeInTheDocument();
		});

		it('creates a vMCP from a drop anywhere on the empty canvas', async () => {
			worker.use(
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}`, () => HttpResponse.json(slack)),
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}/servers`, () =>
					HttpResponse.json({ items: [] })
				)
			);
			await renderPageWithEntries([componentEntry, slack], true, []);

			const canvas = await page.getByCSS('[data-vmcp-canvas]').element();
			const rect = canvas.getBoundingClientRect();
			const el = await panelCard('Slack').element();
			if (!(el instanceof HTMLElement)) throw new Error('Expected an HTMLElement');
			el.setPointerCapture = () => {};
			const from = el.getBoundingClientRect();
			const start = { x: from.left + from.width / 2, y: from.top + from.height / 2 };
			const to = { x: rect.left + 16, y: rect.top + 16 };
			el.dispatchEvent(
				new PointerEvent('pointerdown', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: start.x,
					clientY: start.y
				})
			);
			el.dispatchEvent(
				new PointerEvent('pointermove', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: to.x,
					clientY: to.y
				})
			);
			await tick();
			el.dispatchEvent(
				new PointerEvent('pointerup', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: to.x,
					clientY: to.y
				})
			);

			await expect.element(page.getByRole('dialog').getByText('Create vMCP').first()).toBeVisible();
		});
	});
});
