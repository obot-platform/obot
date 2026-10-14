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
		.toHaveTextContent('obot mcp login "https://obot.example/oauth/mcp/login/test"');
	await expect.element(page.getByRole('button', { name: /Copy/ })).toBeVisible();
});

it('excludes browser hints and shell metacharacters from the command', async () => {
	await render(McpLogin, {
		url: "https://obot.example/oauth/mcp/login/test?name=a'b&x=$(echo)&y=%PATH%#`echo`"
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent('obot mcp login "https://obot.example/oauth/mcp/login/test"');
});

it('resolves relative attempt URLs and allows manual regeneration while pending', async () => {
	const onComplete = vi.fn();
	await render(McpLogin, { url: '/oauth/mcp/login/attempt', onComplete });
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${window.location.origin}/oauth/mcp/login/attempt"`);
	expect(onComplete).not.toHaveBeenCalled();
	await expect
		.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
		.toBeEnabled();
	expect(onComplete).not.toHaveBeenCalled();
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
			await expect.element(page.getByText('3', { exact: true })).not.toBeInTheDocument();
			await expect.element(page.getByRole('link', { name: m.mcps_login_install() })).toBeVisible();
			const command = page.getByLabelText(m.mcps_login_command());
			await expect
				.element(command)
				.toHaveTextContent('obot mcp login "https://obot.example/oauth/mcp/login/test"');
			const button = page.getByRole('button', { name: m.mcps_login_refresh(), exact: true });
			await expect.element(button).toBeVisible();
			expect(command.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			expect(button.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			await expect.element(button).toBeEnabled();
			expect(onComplete).not.toHaveBeenCalled();
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
			return consumed
				? new HttpResponse(null, { status: 400 })
				: HttpResponse.json({ expiresIn: 600 });
		})
	);
	await render(McpLogin, { url: '/oauth/mcp/login/stable-attempt', onComplete });
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Checking for valid authentication…');
	await expect
		.element(page.getByRole('button', { name: 'Continue', exact: true }))
		.not.toBeInTheDocument();
	await vi.waitFor(() => expect(polled.mock.calls.length).toBeGreaterThanOrEqual(2));
	expect(onComplete).not.toHaveBeenCalled();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${window.location.origin}/oauth/mcp/login/stable-attempt"`);
	consumed = true;
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	document.dispatchEvent(new Event('visibilitychange'));
	expect(onComplete).toHaveBeenCalledOnce();
});

it('does not treat a failed status request as successful sign-in and recovers automatically', async () => {
	let failed = true;
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/network-error', () =>
			failed ? new HttpResponse(null, { status: 503 }) : HttpResponse.json({ expiresIn: 600 })
		)
	);
	const result = await render(McpLogin, { url: '/oauth/mcp/login/network-error', onComplete });
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent('Unable to check sign-in. Retrying automatically.');
	await expect
		.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
		.toBeEnabled();
	expect(onComplete).not.toHaveBeenCalled();
	failed = false;
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Checking for valid authentication…');
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

it('explains the named component callback requirement in two steps', async () => {
	await render(McpLogin, {
		url: '/oauth/mcp/login/attempt?serverName=Acme+%26+Co',
		serverName: 'Parent vMCP',
		purpose: 'tools'
	});
	await expect
		.element(page.getByRole('heading', { name: 'Sign in to Acme & Co to load its tools' }))
		.toBeVisible();
	await expect.element(page.getByText(m.mcps_login_reason({ name: 'Acme & Co' }))).toBeVisible();
	await expect.element(page.getByRole('link', { name: 'Install the Obot CLI' })).toBeVisible();
	await expect.element(page.getByText('3', { exact: true })).not.toBeInTheDocument();
	await expect
		.element(
			page.getByText('Run this in a terminal on a computer with a web browser:', { exact: true })
		)
		.toBeVisible();
	await expect.element(page.getByText('Parent vMCP', { exact: true })).not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${window.location.origin}/oauth/mcp/login/attempt"`);
});

it('uses the supplied server name for older login URLs', async () => {
	await render(McpLogin, { url: '/oauth/mcp/login/attempt', serverName: 'Example MCP' });
	await expect
		.element(page.getByRole('heading', { name: 'Sign in to Example MCP', exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByText(m.mcps_login_tools_reason({ name: 'Example MCP' })))
		.not.toBeInTheDocument();
});

it('counts down server lifetime and requires a new command after expiration', async () => {
	let elapsed = 0;
	const now = performance.now.bind(performance);
	const clock = vi.spyOn(performance, 'now').mockImplementation(() => now() + elapsed);
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/countdown', () =>
			elapsed >= 65000
				? new HttpResponse(null, { status: 410 })
				: HttpResponse.json({ expiresIn: 65 })
		)
	);
	const result = await render(McpLogin, { url: '/oauth/mcp/login/countdown', onComplete });
	try {
		await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 1:05');
		await expect
			.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
			.toBeEnabled();
		elapsed = 70000;
		document.dispatchEvent(new Event('visibilitychange'));
		await expect.element(page.getByRole('alert')).toHaveTextContent(m.mcps_login_expired());
		await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Copy/ })).not.toBeInTheDocument();
		await expect.element(page.getByRole('status')).toHaveTextContent('Command expired');
		document.dispatchEvent(new Event('visibilitychange'));
		expect(onComplete).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Generate new command', exact: true }).click();
		expect(onComplete).toHaveBeenCalledOnce();
	} finally {
		await result.unmount();
		clock.mockRestore();
	}
});

it('shows an already expired attempt without automatically replacing it and resets for a new URL', async () => {
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/expired', () => new HttpResponse(null, { status: 410 })),
		http.get('*/oauth/mcp/login/fresh', () => HttpResponse.json({ expiresIn: 600 }))
	);
	const result = await render(McpLogin, { url: '/oauth/mcp/login/expired', onComplete });
	await expect.element(page.getByRole('alert')).toHaveTextContent(m.mcps_login_expired());
	expect(onComplete).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Generate new command', exact: true }).click();
	expect(onComplete).toHaveBeenCalledOnce();
	await result.rerender({ url: '/oauth/mcp/login/fresh', onComplete });
	await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 10:00');
	await expect
		.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
		.toBeEnabled();
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${window.location.origin}/oauth/mcp/login/fresh"`);
});

it('checks the owning flow when the first status response is already consumed', async () => {
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/already-consumed', () => new HttpResponse(null, { status: 400 }))
	);
	await render(McpLogin, { url: '/oauth/mcp/login/already-consumed', onComplete });
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	await expect.element(page.getByText('Command expired', { exact: true })).not.toBeInTheDocument();
	await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	document.dispatchEvent(new Event('visibilitychange'));
	expect(onComplete).toHaveBeenCalledOnce();
});

it('shows an unavailable attempt as expired when there is no owning flow to check', async () => {
	worker.use(
		http.get('*/oauth/mcp/login/unavailable', () => new HttpResponse(null, { status: 400 }))
	);
	await render(McpLogin, { url: '/oauth/mcp/login/unavailable' });
	await expect.element(page.getByRole('alert')).toHaveTextContent(m.mcps_login_expired());
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
});

it.each([false, true])(
	'checks completed authentication past the deadline (hidden: %s)',
	async (hidden) => {
		let elapsed = 0;
		const now = performance.now.bind(performance);
		const clock = vi.spyOn(performance, 'now').mockImplementation(() => now() + elapsed);
		const visibility = vi.spyOn(document, 'visibilityState', 'get');
		visibility.mockReturnValue('visible');
		const onComplete = vi.fn();
		let consumed = false;
		worker.use(
			http.get('*/oauth/mcp/login/final-check', () =>
				consumed ? new HttpResponse(null, { status: 400 }) : HttpResponse.json({ expiresIn: 60 })
			)
		);
		const result = await render(McpLogin, { url: '/oauth/mcp/login/final-check', onComplete });
		try {
			await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 1:00');
			if (hidden) {
				visibility.mockReturnValue('hidden');
				document.dispatchEvent(new Event('visibilitychange'));
			}
			consumed = true;
			elapsed = 61000;
			visibility.mockReturnValue('visible');
			document.dispatchEvent(new Event('visibilitychange'));
			await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
			await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
		} finally {
			await result.unmount();
			clock.mockRestore();
			visibility.mockRestore();
		}
	}
);

it('retries a status failure after the cached deadline without replacing the command', async () => {
	let elapsed = 0;
	const now = performance.now.bind(performance);
	const clock = vi.spyOn(performance, 'now').mockImplementation(() => now() + elapsed);
	let status = 200;
	const requests = vi.fn(() =>
		status === 200 ? HttpResponse.json({ expiresIn: 60 }) : new HttpResponse(null, { status })
	);
	worker.use(http.get('*/oauth/mcp/login/final-retry', requests));
	const onComplete = vi.fn();
	const result = await render(McpLogin, { url: '/oauth/mcp/login/final-retry', onComplete });
	try {
		await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 1:00');
		elapsed = 61000;
		status = 503;
		document.dispatchEvent(new Event('visibilitychange'));
		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('Unable to check sign-in. Retrying automatically.');
		await expect
			.element(page.getByText('Command expired', { exact: true }))
			.not.toBeInTheDocument();
		expect(onComplete).not.toHaveBeenCalled();
		status = 400;
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	} finally {
		await result.unmount();
		clock.mockRestore();
	}
});

it('resumes polling when a manual refresh overlapping a consumed response fails', async () => {
	let returnConsumed!: () => void;
	const statusResponse = new Promise<void>((resolve) => {
		returnConsumed = resolve;
	});
	let rejectRefresh!: (error: Error) => void;
	const manualRefresh = new Promise<void>((_, reject) => {
		rejectRefresh = reject;
	});
	const polled = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/overlapping-refresh', async () => {
			polled();
			await statusResponse;
			return new HttpResponse(null, { status: 400 });
		})
	);
	const onComplete = vi
		.fn()
		.mockImplementationOnce(() => manualRefresh)
		.mockResolvedValue(undefined);
	const result = await render(McpLogin, {
		url: '/oauth/mcp/login/overlapping-refresh',
		onComplete
	});
	try {
		await vi.waitFor(() => expect(polled).toHaveBeenCalledOnce());
		await page.getByRole('button', { name: 'Generate new command', exact: true }).click();
		expect(onComplete).toHaveBeenCalledOnce();
		returnConsumed();
		// Allow the outstanding status response to reach the component while refresh is pending.
		await new Promise((resolve) => setTimeout(resolve, 100));
		expect(onComplete).toHaveBeenCalledOnce();
		rejectRefresh(new Error('Temporary authentication check failure'));
		await vi.waitFor(() => expect(onComplete).toHaveBeenCalledTimes(2), { timeout: 3500 });
		expect(polled).toHaveBeenCalledTimes(2);
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	} finally {
		await result.unmount();
	}
}, 5000);
