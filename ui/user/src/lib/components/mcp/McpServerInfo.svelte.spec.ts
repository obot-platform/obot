import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import McpServerInfo from './McpServerInfo.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it.each(['deployed server', 'catalog entry'])(
	'shows required environment and header fields for a %s',
	async (kind) => {
		const options = {
			id: 'test-server',
			name: 'Test server',
			userID: 'user-1',
			runtime: 'remote' as const,
			manifest: {
				config: [
					{ key: 'TOKEN', name: 'Access token', usage: 'env' as const, required: true },
					{ key: 'X-API-Key', name: 'API key', usage: 'header' as const, required: true },
					{ key: 'OPTIONAL_ENV', name: 'Optional environment', usage: 'env' as const },
					{ key: 'X-Optional', name: 'Optional header', usage: 'header' as const }
				].map((field) => ({
					description: '',
					sensitive: false,
					value: '',
					required: false,
					...field
				}))
			}
		};
		const entry =
			kind === 'deployed server' ? createMCPCatalogServer(options) : createMCPCatalogEntry(options);
		await render(McpServerInfo, { entry });

		await expect.element(page.getByText('Required Configuration', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Access token, API key', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Optional environment')).not.toBeInTheDocument();
		await expect.element(page.getByText('Optional header')).not.toBeInTheDocument();
	}
);
