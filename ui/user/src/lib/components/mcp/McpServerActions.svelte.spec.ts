import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import McpServerActions from './McpServerActions.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => ({
	...(await importOriginal<typeof import('$app/navigation')>()),
	goto: vi.fn()
}));

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
