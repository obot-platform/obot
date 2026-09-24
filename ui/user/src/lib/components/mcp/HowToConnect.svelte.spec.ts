import { preparePageData } from '../../../tests/helpers/pageData';
import HowToConnect from './HowToConnect.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const url = 'https://obot.example/mcp-connect/test';

it('installs the CLI mapping for localhost callbacks', async () => {
	await preparePageData();
	await render(HowToConnect, { id: 'vercel', displayName: 'Vercel', url, localhostCallback: true });
	await expect
		.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
		.toHaveAttribute('href', 'https://docs.obot.ai/installation/cli-setup');
	const cursorLink = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(cursorLink).toBeVisible();
	const cursor = new URL(cursorLink.element().getAttribute('href')!);
	expect(JSON.parse(atob(cursor.searchParams.get('config')!))).toEqual({
		command: 'obot',
		args: ['mcp', 'connect', url]
	});
	const vscode = page
		.getByRole('link', { name: /Add to VS Code$/ })
		.element()
		.getAttribute('href')!;
	expect(JSON.parse(decodeURIComponent(vscode.split('?')[1]))).toEqual({
		name: 'Vercel',
		type: 'stdio',
		command: 'obot',
		args: ['mcp', 'connect', url]
	});
	await expect
		.element(page.getByCSS('#command-claude'))
		.toHaveValue(`claude mcp add --transport stdio "vercel" -- obot mcp connect "${url}"`);
	await expect
		.element(page.getByCSS('#command-codex'))
		.toHaveValue(`codex mcp add "vercel" -- obot mcp connect "${url}"`);
	await expect
		.element(page.getByRole('button', { name: 'Preconfigure server' }))
		.not.toBeInTheDocument();
});

it('keeps HTTP installation for connections without localhost callbacks', async () => {
	await preparePageData();
	await render(HowToConnect, { id: 'remote', displayName: 'Remote', url });
	const cursorLink = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(cursorLink).toBeVisible();
	const cursor = new URL(cursorLink.element().getAttribute('href')!);
	expect(JSON.parse(atob(cursor.searchParams.get('config')!))).toEqual({ type: 'http', url });
	await expect
		.element(page.getByCSS('#command-claude'))
		.toHaveValue(`claude mcp add --transport http "remote" "${url}"`);
	await expect
		.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
		.not.toBeInTheDocument();
});

it('includes custom callback paths in install links, commands, and JSON', async () => {
	await preparePageData();
	await render(HowToConnect, {
		id: 'custom',
		displayName: 'Custom',
		url,
		localhostCallback: true,
		callbackPaths: ['/custom/callback', '/oauth/callback', '/custom/callback']
	});
	const args = [
		'mcp',
		'connect',
		url,
		'--callback-path',
		'/custom/callback',
		'--callback-path',
		'/oauth/callback'
	];
	const cursorLink = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(cursorLink).toBeVisible();
	const cursor = new URL(cursorLink.element().getAttribute('href')!);
	expect(JSON.parse(atob(cursor.searchParams.get('config')!))).toEqual({ command: 'obot', args });
	const vscode = page
		.getByRole('link', { name: /Add to VS Code$/ })
		.element()
		.getAttribute('href')!;
	expect(JSON.parse(decodeURIComponent(vscode.split('?')[1]))).toEqual({
		name: 'Custom',
		type: 'stdio',
		command: 'obot',
		args
	});
	await expect
		.element(page.getByCSS('#command-codex'))
		.toHaveValue(
			`codex mcp add "custom" -- obot mcp connect "${url}" --callback-path '/custom/callback' --callback-path '/oauth/callback'`
		);
	await expect
		.element(page.getByCSS('#command-claude'))
		.toHaveValue(
			`claude mcp add --transport stdio "custom" -- obot mcp connect "${url}" --callback-path '/custom/callback' --callback-path '/oauth/callback'`
		);
	await page.getByText('Connect with the Obot CLI (localhost OAuth)', { exact: true }).click();
	await expect
		.element(page.getByCSS('#local-mcp-config-custom'))
		.toHaveTextContent(
			JSON.stringify({ mcpServers: { custom: { command: 'obot', args } } }, null, 2).replace(
				/\s+/g,
				' '
			)
		);
});
