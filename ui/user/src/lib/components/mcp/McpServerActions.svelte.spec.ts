import { mcpServersAndEntries } from '$lib/stores';
import { preparePageData } from '../../../tests/helpers/pageData';
import { createMcpServerDetailsFixtures } from '../../../tests/mocks/data';
import McpServerActions from './McpServerActions.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

async function renderActions(canConnect: boolean | undefined, hideActions = false) {
	await preparePageData();
	const server = {
		...fixtures.serverSingle,
		canConnect
	};
	mcpServersAndEntries.current = {
		entries: [],
		servers: canConnect === undefined ? [] : [server],
		userInstances: [],
		userConfiguredServers: canConnect === undefined ? [] : [server],
		loading: false,
		lastFetched: Date.now(),
		isInitialized: true
	};
	return render(McpServerActions, { server, hideActions });
}

async function renderEntryActions() {
	await preparePageData();
	const entry = {
		...fixtures.entrySingle,
		canConnect: true
	};
	mcpServersAndEntries.current = {
		entries: [entry],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: Date.now(),
		isInitialized: true
	};
	await render(McpServerActions, { entry });
	return entry;
}

describe('McpServerActions tester action', () => {
	it('shows an accessible canonical Test link for a connection-authorized deployment', async () => {
		await renderActions(true);
		const link = page.getByRole('link', {
			name: `Test ${fixtures.serverSingle.manifest.name}`,
			exact: true
		});
		await expect.element(link).toBeVisible();
		await expect
			.element(link)
			.toHaveAttribute('href', `/mcp-servers/test/${fixtures.serverSingle.id}`);
	});

	it('shows Test whenever Connect is present and mirrors its disabled state', async () => {
		await renderActions(false);
		await expect.element(page.getByRole('button', { name: /^Test / })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /^Test / })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeDisabled();

		await renderActions(undefined);
		await expect.element(page.getByRole('link', { name: /^Test / })).toBeVisible();
	});

	it('starts the preconfigure flow when an entry has no deployment', async () => {
		const entry = await renderEntryActions();

		await expect
			.element(page.getByRole('button', { name: `Test ${entry.manifest.name}`, exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
		await page.getByRole('button', { name: `Test ${entry.manifest.name}`, exact: true }).click();

		await expect.element(page.getByText('Connect To Server', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('This will begin the initial setup process for this server.'))
			.toBeVisible();
	});

	it('keeps Test visible on admin headers that hide connection and management actions', async () => {
		await renderActions(true, true);

		await expect.element(page.getByRole('link', { name: /^Test / })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Connect', exact: true }))
			.not.toBeInTheDocument();
	});
});
