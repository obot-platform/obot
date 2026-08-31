import { mcpServersAndEntries } from '$lib/stores';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { createMcpServerDetailsFixtures } from '../../../../tests/mocks/data';
import McpTesterAction from './McpTesterAction.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

async function renderAction(
	canConnect: boolean | undefined,
	authorizedInStore = false,
	overrides: Record<string, unknown> = {}
) {
	await preparePageData();
	const server = {
		...fixtures.serverSingle,
		...overrides,
		canConnect
	};
	mcpServersAndEntries.current = {
		entries: [],
		servers: authorizedInStore ? [{ ...server, canConnect: true }] : [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: Date.now(),
		isInitialized: true
	};
	return render(McpTesterAction, { server });
}

describe('McpTesterAction', () => {
	it('shows a canonical link for an explicit deployment connection grant', async () => {
		await renderAction(true);
		const link = page.getByRole('link', {
			name: `Test ${fixtures.serverSingle.manifest.name}`,
			exact: true
		});

		await expect.element(link).toBeVisible();
		await expect
			.element(link)
			.toHaveAttribute('href', `/mcp-servers/test/${fixtures.serverSingle.id}`);
	});

	it('uses authorization-filtered store data only when the deployment omits permission data', async () => {
		await renderAction(undefined, true);
		await expect.element(page.getByRole('link', { name: /^Test / })).toBeVisible();
	});

	it('hides for denied, unknown, deleted, and composite-child deployments', async () => {
		await renderAction(false, true);
		await expect.element(page.getByRole('link', { name: /^Test / })).not.toBeInTheDocument();

		await renderAction(undefined);
		await expect.element(page.getByRole('link', { name: /^Test / })).not.toBeInTheDocument();

		await renderAction(true, false, { deleted: true });
		await expect.element(page.getByRole('link', { name: /^Test / })).not.toBeInTheDocument();

		await renderAction(true, false, { compositeName: 'composite-parent' });
		await expect.element(page.getByRole('link', { name: /^Test / })).not.toBeInTheDocument();
	});

	it('supports a forced entry action before a deployment exists', async () => {
		await preparePageData();
		const ontest = vi.fn();
		render(McpTesterAction, {
			entry: fixtures.entrySingle,
			forceVisible: true,
			ontest
		});

		await page
			.getByRole('button', { name: `Test ${fixtures.entrySingle.manifest.name}`, exact: true })
			.click();
		expect(ontest).toHaveBeenCalledOnce();
	});
});
