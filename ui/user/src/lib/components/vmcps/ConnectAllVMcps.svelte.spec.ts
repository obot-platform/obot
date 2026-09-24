import { AiClient, COMMON_AI_CLIENTS } from '$lib/services/user/constants';
import { buildConnectAllSnippets } from '$lib/services/vmcps/utils';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import ConnectAllVMcps from './ConnectAllVMcps.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function connections() {
	const remote = createVMCP({
		id: 'remote',
		displayName: 'Remote',
		links: { connectURL: 'https://obot.example/mcp-connect/remote' }
	});
	const local = createVMCP({
		id: 'local',
		displayName: 'Local',
		links: { connectURL: 'https://obot.example/mcp-connect/local' }
	});
	local.components![0].catalogEntry.manifest.remoteConfig = { localhostCallbackEnabled: true };
	return [remote, local];
}
const http = { type: 'http', url: 'https://obot.example/mcp-connect/remote' };
const stdio = {
	type: 'stdio',
	command: 'obot',
	args: ['mcp', 'connect', 'https://obot.example/mcp-connect/local']
};

for (const client of COMMON_AI_CLIENTS) {
	it(`builds mixed connection configurations for ${client.id}`, () => {
		const snippets = buildConnectAllSnippets(client.id, connections(), false);
		if (client.id === AiClient.Codex) {
			expect(snippets[0].value).toBe(
				'[mcp_servers.Remote]\nurl = "https://obot.example/mcp-connect/remote"\n\n[mcp_servers.Local]\ncommand = "obot"\nargs = ["mcp","connect","https://obot.example/mcp-connect/local"]'
			);
		} else {
			const key = client.id === AiClient.VSCode ? 'servers' : 'mcpServers';
			expect(JSON.parse(snippets[0].value)).toEqual({ [key]: { Remote: http, Local: stdio } });
		}
	});
}
it('uses exact command allowlisting for Claude Enterprise', () => {
	const snippets = buildConnectAllSnippets(AiClient.Claude, connections(), true);
	expect(JSON.parse(snippets[0].value)).toEqual({
		allowedMcpServers: [{ serverUrl: http.url }, { serverCommand: [stdio.command, ...stdio.args] }]
	});
	expect(JSON.parse(snippets[1].value)).toEqual({ mcpServers: { Remote: http, Local: stdio } });
});
it('preserves duplicate names, skips empty vMCPs, and derives missing URLs', () => {
	const [remote, local] = connections();
	local.displayName = remote.displayName;
	const empty = createVMCP({ id: 'empty', components: [] });
	const missing = createVMCP({ id: 'missing', links: {} });
	const snippets = buildConnectAllSnippets(AiClient.Cursor, [remote, local, empty, missing], false);
	expect(JSON.parse(snippets[0].value)).toEqual({
		mcpServers: {
			Remote: http,
			'Remote (local)': stdio,
			'Issue Tracker vMCP': { type: 'http', url: `${window.location.origin}/mcp-connect/missing` }
		}
	});
});
for (const local of [true, false]) {
	it(`shows CLI installation guidance only when required: ${local}`, async () => {
		await preparePageData();
		const vmcps = local ? connections() : connections().slice(0, 1);
		const result = await render(ConnectAllVMcps, { vmcps });
		result.component.open(COMMON_AI_CLIENTS.find((c) => c.id === AiClient.Cursor)!);
		await expect.element(page.getByRole('dialog')).toBeVisible();
		const link = page.getByRole('link', { name: 'Install the Obot CLI' });
		if (local) {
			await expect.element(link).toBeVisible();
			await expect
				.element(page.getByCSS('#connect-all-mcp-json-cursor-mcp-json'))
				.toHaveTextContent(
					JSON.stringify({ mcpServers: { Remote: http, Local: stdio } }, null, 2).replace(
						/\s+/g,
						' '
					)
				);
		} else {
			await expect.element(link).not.toBeInTheDocument();
			await expect
				.element(page.getByCSS('#connect-all-mcp-json-cursor-mcp-json'))
				.toHaveTextContent(
					JSON.stringify({ mcpServers: { Remote: http } }, null, 2).replace(/\s+/g, ' ')
				);
		}
	});
}

it('includes component callback paths in bulk JSON, TOML, and enterprise allowlists', () => {
	const vmcps = connections();
	const local = vmcps[1];
	const component = local.components![0];
	component.catalogEntry.manifest.remoteConfig!.localhostCallbackPath = '/custom/callback';
	const second = structuredClone(component);
	second.id = 'second';
	second.catalogEntry.manifest.remoteConfig!.localhostCallbackPath = '';
	local.components!.push(second, structuredClone(component));
	const args = [
		...stdio.args,
		'--callback-path',
		'/custom/callback',
		'--callback-path',
		'/oauth/callback'
	];
	for (const client of COMMON_AI_CLIENTS) {
		const value = buildConnectAllSnippets(client.id, vmcps, false)[0].value;
		if (client.id === AiClient.Codex) {
			expect(value).toContain(`args = ${JSON.stringify(args)}`);
		} else {
			const servers = JSON.parse(value)[client.id === AiClient.VSCode ? 'servers' : 'mcpServers'];
			expect(servers.Local.args).toEqual(args);
			expect(servers.Remote).toEqual(http);
		}
	}
	const policy = JSON.parse(buildConnectAllSnippets(AiClient.Claude, vmcps, true)[0].value);
	expect(policy.allowedMcpServers[1]).toEqual({ serverCommand: ['obot', ...args] });
});
