import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import McpServerActions from './McpServerActions.svelte';
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn()
}));

describe('MCP server OAuth setup link', () => {
	it('opens configured credentials with a clear action', async () => {
		await preparePageData();
		worker.use(
			http.get('/api/mcp-catalogs/default/entries/salesforce/oauth-credentials', () =>
				HttpResponse.json({ configured: true, clientID: 'salesforce-client' })
			)
		);
		const entry = createMCPCatalogEntry({
			id: 'salesforce',
			name: 'Salesforce',
			runtime: 'remote',
			oauthCredentialConfigured: true,
			manifest: { remoteConfig: { fixedURL: 'https://example.com/mcp', staticOAuthRequired: true } }
		});

		render(McpServerActions, { entry, promptOAuthConfig: true, hideActions: true });
		await expect.element(page.getByRole('button', { name: 'Clear Credentials' })).toBeVisible();
	});
});

for (const kind of ['entry', 'server'] as const) {
	for (const enabled of [true, false]) {
		it(`shows ${enabled ? 'CLI setup' : 'HTTP URL'} after launching a ${kind}`, async () => {
			await preparePageData();
			const resource =
				kind === 'entry'
					? createMCPCatalogEntry({ id: 'local-entry', name: 'Local', runtime: 'remote' })
					: createMCPCatalogServer({
							id: 'local-server',
							name: 'Local',
							runtime: 'remote',
							userID: 'user-1'
						});
			resource.connectURL = 'https://obot.example/mcp-connect/local';
			resource.manifest.remoteConfig = { localhostCallbackEnabled: enabled };
			await render(McpServerActions, { [kind]: resource, promptInitialLaunch: true });
			if (enabled) {
				await expect
					.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
					.toBeVisible();
				await expect
					.element(page.getByCSS('#server-action-connection-url'))
					.not.toBeInTheDocument();
				await expect
					.element(page.getByCSS('#command-codex'))
					.toHaveValue(
						`codex mcp add "${resource.id}" -- obot mcp connect "${resource.connectURL}"`
					);
			} else {
				await expect
					.element(page.getByCSS('#server-action-connection-url'))
					.toHaveValue(resource.connectURL);
				await expect
					.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
					.not.toBeInTheDocument();
			}
		});
	}
}
