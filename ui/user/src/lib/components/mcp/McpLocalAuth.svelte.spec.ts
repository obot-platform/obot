import { worker } from '../../../tests/mocks/worker';
import McpLocalAuth from './McpLocalAuth.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('keeps a compact row until Authenticate is clicked, then shows status beside the server name', async () => {
	const polled = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/compact', () => {
			polled();
			return HttpResponse.json({ expiresIn: 600 });
		})
	);
	await render(McpLocalAuth, {
		url: '/oauth/mcp/login/compact?serverName=Vercel',
		serverName: 'Parent vMCP',
		onComplete: vi.fn()
	});
	await expect.element(page.getByText('Vercel', { exact: true })).toBeVisible();
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
	expect(polled).not.toHaveBeenCalled();
	const button = page.getByRole('button', { name: 'Authenticate', exact: true });
	await expect.element(button).toBeVisible();
	const name = page.getByText('Vercel', { exact: true }).element().getBoundingClientRect();
	const action = button.element().getBoundingClientRect();
	expect(name.top).toBeLessThan(action.bottom);
	expect(name.bottom).toBeGreaterThan(action.top);
	await button.click();
	await expect.element(page.getByLabelText('Authentication command')).toBeVisible();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Checking for valid authentication…');
	const status = page.getByRole('status').element().getBoundingClientRect();
	expect(status.top).toBeLessThan(
		page.getByText('Vercel', { exact: true }).element().getBoundingClientRect().bottom
	);
	await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 10:00');
	await expect.element(page.getByText('This page', { exact: false })).not.toBeInTheDocument();
	expect(await page.getByRole('listitem').elements()).toHaveLength(2);
});

it.each([390, 1280])(
	'keeps the emphasized timer and refresh action on one line at %ipx',
	async (width) => {
		await page.viewport(width, 900);
		try {
			worker.use(http.get('*/oauth/mcp/login/layout', () => HttpResponse.json({ expiresIn: 600 })));
			await render(McpLocalAuth, {
				url: '/oauth/mcp/login/layout',
				serverName: 'Vercel',
				onComplete: vi.fn()
			});
			await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
			const timer = page.getByRole('timer');
			await expect.element(timer).toHaveTextContent('Command expires in 10:00');
			const timerBox = timer.element().getBoundingClientRect();
			const refresh = page
				.getByRole('button', { name: 'Generate new command', exact: true })
				.element()
				.getBoundingClientRect();
			expect(timerBox.top).toBeLessThan(refresh.bottom);
			expect(timerBox.bottom).toBeGreaterThan(refresh.top);
			expect(refresh.right).toBeLessThanOrEqual(width);
			await expect.element(timer).toHaveClass('text-base-content');
		} finally {
			await page.viewport(1280, 720);
		}
	}
);
