import { preparePageData } from '../../../tests/helpers/pageData';
import HowToConnect from './HowToConnect.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const url = 'https://obot.example/mcp-connect/test';

it('uses HTTP for install links and client commands', async () => {
	await preparePageData();
	await render(HowToConnect, { id: 'vercel', displayName: 'Vercel', url });
	const cursorLink = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(cursorLink).toBeVisible();
	const cursor = new URL(cursorLink.element().getAttribute('href')!);
	expect(JSON.parse(atob(cursor.searchParams.get('config')!))).toEqual({ type: 'http', url });
	const vscode = page
		.getByRole('link', { name: /Add to VS Code$/ })
		.element()
		.getAttribute('href')!;
	expect(JSON.parse(decodeURIComponent(vscode.split('?')[1]))).toEqual({
		name: 'Vercel',
		type: 'http',
		url
	});
	await expect
		.element(page.getByCSS('#command-claude'))
		.toHaveValue(`claude mcp add --transport http "vercel" "${url}"`);
	await expect
		.element(page.getByCSS('#command-codex'))
		.toHaveValue(`codex mcp add "vercel" --url "${url}"`);
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

it('launches the existing preconfigure flow for localhost OAuth', async () => {
	await preparePageData();
	const onLaunch = vi.fn();
	await render(HowToConnect, {
		id: 'local',
		displayName: 'Local',
		url,
		onLaunch
	});
	await page.getByRole('button', { name: 'Preconfigure server' }).click();
	expect(onLaunch).toHaveBeenCalledOnce();
});
