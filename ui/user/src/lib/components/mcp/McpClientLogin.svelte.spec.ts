import { worker } from '../../../tests/mocks/worker';
import McpClientLogin from './McpClientLogin.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('resumes only after the original request is authenticated', async () => {
	const onComplete = vi.fn();
	const requests = vi.fn();
	const attempt = `${window.location.origin}/oauth/mcp/login/ui-local-client`;
	let completed = false;
	worker.use(
		http.get('*/oauth/mcp/login-request/client-request', () => {
			requests();
			return HttpResponse.json({ authURL: completed ? '' : attempt });
		})
	);
	await render(McpClientLogin, { authRequestID: 'client-request', onComplete });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${attempt}'`);
	await expect.element(page.getByRole('link', { name: 'Install the Obot CLI' })).toBeVisible();
	expect(onComplete).not.toHaveBeenCalled();
	await expect
		.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
		.toBeDisabled();
	expect(requests).toHaveBeenCalledOnce();
	expect(onComplete).not.toHaveBeenCalled();
	completed = true;
	worker.use(
		http.get('*/oauth/mcp/login/ui-local-client', () => new HttpResponse(null, { status: 400 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
});

it('requires manual replacement of expired attempts without completing the connection', async () => {
	let checks = 0;
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login-request/retry-request', () =>
			HttpResponse.json({
				authURL: `${window.location.origin}/oauth/mcp/login/attempt-${++checks}`
			})
		)
	);
	await render(McpClientLogin, { authRequestID: 'retry-request', onComplete });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${window.location.origin}/oauth/mcp/login/attempt-1'`);
	worker.use(
		http.get('*/oauth/mcp/login/attempt-1', () => new HttpResponse(null, { status: 410 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await expect.element(page.getByRole('status')).toHaveTextContent('Command expired');
	expect(checks).toBe(1);
	await page.getByRole('button', { name: 'Generate new command', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${window.location.origin}/oauth/mcp/login/attempt-2'`);
	expect(onComplete).not.toHaveBeenCalled();
	// Another expired command can be replaced without claiming success.
	worker.use(
		http.get('*/oauth/mcp/login/attempt-2', () => new HttpResponse(null, { status: 410 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await expect.element(page.getByRole('status')).toHaveTextContent('Command expired');
	await page.getByRole('button', { name: 'Generate new command', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${window.location.origin}/oauth/mcp/login/attempt-3'`);
	expect(onComplete).not.toHaveBeenCalled();
});

it('allows retry after a failed authentication check', async () => {
	let failed = true;
	worker.use(
		http.get('*/oauth/mcp/login-request/failure', () =>
			failed
				? HttpResponse.json({ message: 'Authentication check failed' }, { status: 500 })
				: HttpResponse.json({ authURL: `${window.location.origin}/oauth/mcp/login/retried` })
		)
	);
	await render(McpClientLogin, { authRequestID: 'failure', onComplete: vi.fn() });
	await expect.element(page.getByRole('alert')).toBeVisible();
	failed = false;
	await page.getByRole('button', { name: 'Retry', exact: true }).click();
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${window.location.origin}/oauth/mcp/login/retried'`);
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
});

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});

it('identifies the authenticating server in a client-initiated login', async () => {
	worker.use(
		http.get('*/oauth/mcp/login-request/named', () =>
			HttpResponse.json({
				authURL: `${window.location.origin}/oauth/mcp/login/named?serverName=Vercel`
			})
		)
	);
	await render(McpClientLogin, { authRequestID: 'named', onComplete: vi.fn() });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect.element(page.getByText('Vercel', { exact: true })).toBeVisible();
	await expect.element(page.getByText(/Vercel requires a localhost callback/)).toBeVisible();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login '${window.location.origin}/oauth/mcp/login/named'`);
});
