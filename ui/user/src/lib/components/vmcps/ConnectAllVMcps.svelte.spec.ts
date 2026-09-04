import { AiClient, COMMON_AI_CLIENTS } from '$lib/services/user/constants';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import ConnectAllVMcps from './ConnectAllVMcps.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const cursor = COMMON_AI_CLIENTS.find((client) => client.id === AiClient.Cursor)!;
const claude = COMMON_AI_CLIENTS.find((client) => client.id === AiClient.Claude)!;

const vmcp = createVMCP({
	id: 'vmcp-1',
	displayName: 'Issue Tracker vMCP'
});

async function renderDialog(vmcps = [vmcp]) {
	await preparePageData();
	return render(ConnectAllVMcps, { vmcps });
}

describe('ConnectAllVMcps.svelte', () => {
	it('shows Cursor config for vMCPs that have a connection URL', async () => {
		const result = await renderDialog();
		result.component.open(cursor);

		await expect.element(page.getByText('Connect All vMCPs')).toBeVisible();
		await expect.element(page.getByText('~/.cursor/mcp.json')).toBeVisible();
		await expect.element(page.getByText(/Issue Tracker vMCP/)).toBeVisible();
		await expect.element(page.getByText('/mcp-connect/vmcp-1')).toBeVisible();
	});

	it('shows an empty state when no vMCP has a connection URL', async () => {
		const result = await renderDialog([createVMCP({ id: 'vmcp-2', links: {} })]);
		result.component.open(cursor);

		await expect
			.element(page.getByText('No vMCPs currently have a connection URL to copy.'))
			.toBeVisible();
		await expect.element(page.getByText('/mcp-connect/')).not.toBeInTheDocument();
	});

	it('shows Claude enterprise and mcp.json tabs for admins', async () => {
		const result = await renderDialog();
		result.component.open(claude);

		await expect.element(page.getByRole('tab', { name: 'Claude Enterprise' })).toBeVisible();
		await expect.element(page.getByRole('tab', { name: 'mcp.json' })).toBeVisible();
		await expect
			.element(page.getByText('Admin Settings > Claude Code > Managed settings'))
			.toBeVisible();

		await page.getByRole('tab', { name: 'mcp.json' }).click();
		await expect.element(page.getByText('.mcp.json')).toBeVisible();
		await expect.element(page.getByText('~/.claude.json')).toBeVisible();
	});
});
