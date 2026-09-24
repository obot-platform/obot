import LocalMcpConnection from './LocalMcpConnection.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('generates a local STDIO configuration for the selected connection', async () => {
	await render(LocalMcpConnection, {
		id: 'vmcpi1',
		url: 'https://obot.example/mcp-connect/vmcpi1'
	});
	await page.getByText('Connect with the Obot CLI (localhost OAuth)', { exact: true }).click();
	const expected = JSON.stringify(
		{
			mcpServers: {
				vmcpi1: {
					command: 'obot',
					args: ['mcp', 'connect', 'https://obot.example/mcp-connect/vmcpi1']
				}
			}
		},
		null,
		2
	).replace(/\s+/g, ' ');
	await expect.element(page.getByCSS('#local-mcp-config-vmcpi1')).toHaveTextContent(expected);
});
