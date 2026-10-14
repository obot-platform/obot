import { mcpServersAndEntries } from '$lib/stores';
import { preparePageData } from '../../../tests/helpers/pageData';
import { createMcpServerDetailsFixtures } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import ConnectToServer from './ConnectToServer.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

async function startServerSetup(server: typeof fixtures.serverSingle, onConnect: () => void) {
	const entry = structuredClone(fixtures.entrySingle);
	entry.manifest.remoteConfig = {
		fixedURL: server.manifest.remoteConfig?.url,
		localhostCallbackEnabled: true,
		localhostCallbackPath: server.manifest.remoteConfig?.localhostCallbackPath
	};
	const createdServer = { ...server, configured: true, canConnect: true };
	mcpServersAndEntries.current = {
		entries: [entry],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: Date.now(),
		isInitialized: true
	};
	const createRequest = vi.fn();
	worker.use(
		http.post('/api/mcp-servers', async ({ request }) => {
			createRequest(await request.json());
			return HttpResponse.json(createdServer);
		}),
		http.post(`/api/mcp-servers/${server.id}/configure`, () => HttpResponse.json(createdServer)),
		http.post(`/api/mcp-servers/${server.id}/launch`, () => HttpResponse.json({}))
	);
	const result = await render(ConnectToServer, { onConnect });
	result.component.setupNewInstance(entry);
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'Authenticate', exact: true }))
		.toBeVisible();
	expect(createRequest).toHaveBeenCalledWith({ catalogEntryID: entry.id, manifest: {} });
}

describe('ConnectToServer setup completion', () => {
	it('finishes an existing deployment without showing a direct connection URL', async () => {
		await preparePageData();
		const onConnect = vi.fn();
		const entry = { ...fixtures.entrySingle, connectURL: 'https://obot.example/mcp-connect/entry' };
		const server = {
			...fixtures.serverSingle,
			connectURL: 'https://obot.example/mcp-connect/server'
		};
		const result = await render(ConnectToServer, { onConnect });

		result.component.open({ entry, server });

		expect(onConnect).toHaveBeenCalledWith({ entry, server, instance: undefined });
		await expect.element(page.getByCSS('#connect-to-server-dialog')).not.toBeInTheDocument();
		await expect.element(page.getByLabelText('Connection URL')).not.toBeInTheDocument();
	});

	it('returns an OAuth deployment without opening connection instructions', async () => {
		await preparePageData();
		mcpServersAndEntries.current = {
			entries: [fixtures.entrySingle],
			servers: [],
			userInstances: [],
			userConfiguredServers: [],
			loading: false,
			lastFetched: Date.now(),
			isInitialized: true
		};
		const createdServer = {
			...fixtures.serverSingle,
			configured: true,
			canConnect: true
		};
		const createRequest = vi.fn();
		let oauthChecks = 0;
		worker.use(
			http.post('/api/mcp-servers', async ({ request }) => {
				createRequest(await request.json());
				return HttpResponse.json(createdServer);
			}),
			http.post(`/api/mcp-servers/${createdServer.id}/configure`, () =>
				HttpResponse.json(createdServer)
			),
			http.post(`/api/mcp-servers/${createdServer.id}/launch`, () => HttpResponse.json({})),
			http.get(`/api/mcp-servers/${createdServer.id}/oauth-url`, () => {
				oauthChecks += 1;
				return HttpResponse.json({
					oauthURL: oauthChecks === 1 ? 'https://auth.example.com/authorize' : ''
				});
			})
		);
		const onComplete = vi.fn();
		const result = await render(ConnectToServer);

		result.component.setupNewInstance(fixtures.entrySingle, onComplete);
		await page.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(
			() => {
				expect(createRequest).toHaveBeenCalledWith({
					catalogEntryID: fixtures.entrySingle.id,
					manifest: {}
				});
				expect(onComplete).toHaveBeenCalledWith({
					server: createdServer,
					entry: fixtures.entrySingle,
					instance: undefined
				});
				expect(oauthChecks).toBe(2);
			},
			{ timeout: 1800 }
		);
		await expect.element(page.getByCSS('#connect-to-server-dialog')).not.toBeInTheDocument();
	}, 4000);
});

it('does not advertise a direct URL for a localhost callback server', async () => {
	await preparePageData();
	const entry = structuredClone(fixtures.entrySingle);
	Object.assign(entry, { connectURL: 'https://obot.example/mcp-connect/vercel' });
	entry.manifest.remoteConfig = {
		fixedURL: 'https://mcp.example.com',
		localhostCallbackEnabled: true
	};
	const result = await render(ConnectToServer);
	result.component.open({ entry });
	await expect
		.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
		.not.toBeInTheDocument();
	await expect.element(page.getByText('Connection URL', { exact: true })).not.toBeInTheDocument();
});

it('does not offer direct installation with a custom catalog callback path', async () => {
	await preparePageData();
	const entry = structuredClone(fixtures.entrySingle);
	Object.assign(entry, { connectURL: 'https://obot.example/mcp-connect/custom' });
	entry.manifest.remoteConfig = {
		localhostCallbackEnabled: true,
		localhostCallbackPath: '/custom/callback'
	};
	const result = await render(ConnectToServer);
	result.component.open({ entry });
	const link = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(link).not.toBeInTheDocument();
});

it('shows CLI login when a deployed localhost server needs authentication', async () => {
	await preparePageData();
	const attemptURL = `${window.location.origin}/oauth/mcp/login/ui-attempt`;
	let checks = 0;
	worker.use(
		http.get('*/api/*/oauth-url', () =>
			HttpResponse.json({ oauthURL: ++checks === 1 ? attemptURL : '' })
		)
	);

	const server = structuredClone(fixtures.serverSingle);
	server.manifest.remoteConfig = {
		url: 'https://mcp.example.com',
		localhostCallbackEnabled: true,
		localhostCallbackPath: '/custom/callback'
	};
	const onConnect = vi.fn();
	await startServerSetup(server, onConnect);
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByRole('dialog').getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${attemptURL}"`);
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.not.toBeInTheDocument();
	worker.use(http.get('*/oauth/mcp/login/:state', () => new HttpResponse(null, { status: 400 })));
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(onConnect).toHaveBeenCalledOnce());
});

it.each([390, 1280])('keeps local login usable at %ipx and dismisses it once', async (width) => {
	await page.viewport(width, 900);
	try {
		await preparePageData();
		worker.use(
			http.get('*/api/*/oauth-url', () =>
				HttpResponse.json({ oauthURL: `${window.location.origin}/oauth/mcp/login/ui-attempt` })
			)
		);
		const server = structuredClone(fixtures.serverSingle);
		server.manifest.remoteConfig = {
			url: 'https://mcp.example.com',
			localhostCallbackEnabled: true
		};
		const onConnect = vi.fn();
		await startServerSetup(server, onConnect);
		await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
		const command = page.getByRole('dialog').getByLabelText('Authentication command');
		await expect.element(command).toBeVisible();
		// Wait for the shared dialog's slide-in animation to settle.
		await vi.waitFor(() => {
			const bounds = command.element().getBoundingClientRect();
			expect(bounds.width).toBeGreaterThan(250);
			expect(bounds.left).toBeGreaterThanOrEqual(0);
			expect(bounds.right).toBeLessThanOrEqual(width);
		});
		await userEvent.keyboard('{Escape}');
		await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
		await vi.waitFor(() => expect(onConnect).toHaveBeenCalledOnce());
	} finally {
		await page.viewport(1280, 720);
	}
});

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});

it('stops watching local login when its dialog is dismissed, including an in-flight response', async () => {
	await preparePageData();
	let finish!: () => void;
	const pending = new Promise<void>((resolve) => {
		finish = resolve;
	});
	const polled = vi.fn();
	const oauthChecks = vi.fn();
	worker.use(
		http.get('*/api/*/oauth-url', () => {
			oauthChecks();
			return HttpResponse.json({ oauthURL: `${window.location.origin}/oauth/mcp/login/dismissed` });
		}),
		http.get('*/oauth/mcp/login/dismissed', async () => {
			polled();
			await pending;
			return new HttpResponse(null, { status: 400 });
		})
	);
	const server = structuredClone(fixtures.serverSingle);
	server.manifest.remoteConfig = { url: 'https://mcp.example.com', localhostCallbackEnabled: true };
	const onConnect = vi.fn();
	await startServerSetup(server, onConnect);
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await vi.waitFor(() => expect(polled).toHaveBeenCalledOnce());
	await userEvent.keyboard('{Escape}');
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
	finish();
	await vi.waitFor(() => expect(onConnect).toHaveBeenCalledOnce());
	document.dispatchEvent(new Event('visibilitychange'));
	expect(oauthChecks).toHaveBeenCalledOnce();
	expect(polled).toHaveBeenCalledOnce();
});
