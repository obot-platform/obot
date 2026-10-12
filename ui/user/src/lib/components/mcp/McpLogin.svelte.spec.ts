import { m, locales } from '$lib/i18n';
import { getLocale, overwriteGetLocale } from '$lib/paraglide/runtime';
import { worker } from '../../../tests/mocks/worker';
import McpLogin from './McpLogin.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('shows a copyable command for the specific UI attempt', async () => {
	await render(McpLogin, {
		url: 'https://obot.example/oauth/mcp/login/test'
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent("obot mcp login --url 'https://obot.example/oauth/mcp/login/test'");
	await expect.element(page.getByRole('button', { name: /Copy/ })).toBeVisible();
});

it('quotes shell metacharacters', async () => {
	await render(McpLogin, {
		url: "https://obot.example/oauth/mcp/login/test?name=a'b&x=$(echo)"
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url 'https://obot.example/oauth/mcp/login/test?name=a'"'"'b&x=$(echo)'`
		);
});

it('resolves relative attempt URLs and refreshes only when requested while the attempt is pending', async () => {
	const onComplete = vi.fn();
	await render(McpLogin, { url: '/oauth/mcp/login/attempt', onComplete });
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${window.location.origin}/oauth/mcp/login/attempt'`);
	expect(onComplete).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Get a new sign-in command', exact: true }).click();
	expect(onComplete).toHaveBeenCalledOnce();
});

it.each(locales)(
	'renders localized login instructions and actions in %s on mobile',
	async (locale) => {
		const restoreLocale = getLocale;
		overwriteGetLocale(() => locale);
		await page.viewport(390, 844);
		try {
			const onComplete = vi.fn();
			await render(McpLogin, { url: 'https://obot.example/oauth/mcp/login/test', onComplete });
			await expect
				.element(page.getByText(m.mcps_login_instructions(), { exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByText(m.mcps_login_retry_instructions(), { exact: true }))
				.toBeVisible();
			await expect.element(page.getByRole('link', { name: m.mcps_login_install() })).toBeVisible();
			const command = page.getByLabelText(m.mcps_login_command());
			await expect
				.element(command)
				.toHaveTextContent("obot mcp login --url 'https://obot.example/oauth/mcp/login/test'");
			const button = page.getByRole('button', { name: m.mcps_login_refresh(), exact: true });
			await expect.element(button).toBeVisible();
			expect(command.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			expect(button.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			await button.click();
			expect(onComplete).toHaveBeenCalledOnce();
		} finally {
			overwriteGetLocale(restoreLocale);
			await page.viewport(1280, 720);
		}
	}
);

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});

it('waits on the existing attempt and checks the owning flow only after it is consumed', async () => {
	let consumed = false;
	const polled = vi.fn();
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/stable-attempt', () => {
			polled();
			return new HttpResponse(null, { status: consumed ? 400 : 200 });
		})
	);
	await render(McpLogin, { url: '/oauth/mcp/login/stable-attempt', onComplete });
	await expect.element(page.getByRole('status')).toHaveTextContent('Waiting for sign-in…');
	await expect
		.element(page.getByRole('button', { name: 'Continue', exact: true }))
		.not.toBeInTheDocument();
	await vi.waitFor(() => expect(polled.mock.calls.length).toBeGreaterThanOrEqual(2));
	expect(onComplete).not.toHaveBeenCalled();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url '${window.location.origin}/oauth/mcp/login/stable-attempt'`
		);
	consumed = true;
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	document.dispatchEvent(new Event('visibilitychange'));
	expect(onComplete).toHaveBeenCalledOnce();
});

it('does not treat a failed status request as successful sign-in and recovers automatically', async () => {
	let failed = true;
	const onComplete = vi.fn();
	worker.use(
		http.get(
			'*/oauth/mcp/login/network-error',
			() => new HttpResponse(null, { status: failed ? 503 : 200 })
		)
	);
	const result = await render(McpLogin, { url: '/oauth/mcp/login/network-error', onComplete });
	await expect.element(page.getByRole('alert')).toBeVisible();
	expect(onComplete).not.toHaveBeenCalled();
	failed = false;
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	await expect.element(page.getByRole('status')).toHaveTextContent('Waiting for sign-in…');
	expect(onComplete).not.toHaveBeenCalled();
	await result.unmount();
});

it('ignores an in-flight status response after the login view is removed', async () => {
	const onComplete = vi.fn();
	const polled = vi.fn();
	let finish!: () => void;
	const pending = new Promise<void>((resolve) => {
		finish = resolve;
	});
	worker.use(
		http.get('*/oauth/mcp/login/removed', async () => {
			polled();
			await pending;
			return new HttpResponse(null, { status: 400 });
		})
	);
	const result = await render(McpLogin, { url: '/oauth/mcp/login/removed', onComplete });
	await vi.waitFor(() => expect(polled).toHaveBeenCalledOnce());
	await result.unmount();
	finish();
	document.dispatchEvent(new Event('visibilitychange'));
	expect(onComplete).not.toHaveBeenCalled();
});
