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
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${attempt}'`);
	await expect.element(page.getByRole('link', { name: 'Install the Obot CLI' })).toBeVisible();
	expect(onComplete).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Get a new sign-in command', exact: true }).click();
	await vi.waitFor(() => expect(requests).toHaveBeenCalledTimes(2));
	expect(onComplete).not.toHaveBeenCalled();
	completed = true;
	worker.use(
		http.get('*/oauth/mcp/login/ui-local-client', () => new HttpResponse(null, { status: 400 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
});

it('replaces expired or busy-port attempts without completing the connection', async () => {
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
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url '${window.location.origin}/oauth/mcp/login/attempt-1'`
		);
	worker.use(
		http.get('*/oauth/mcp/login/attempt-1', () => new HttpResponse(null, { status: 400 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url '${window.location.origin}/oauth/mcp/login/attempt-2'`
		);
	expect(onComplete).not.toHaveBeenCalled();
	// A busy callback port can also be recovered explicitly without claiming success.
	await page.getByRole('button', { name: 'Get a new sign-in command', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url '${window.location.origin}/oauth/mcp/login/attempt-3'`
		);
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
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${window.location.origin}/oauth/mcp/login/retried'`);
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
});

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});
