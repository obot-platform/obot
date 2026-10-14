import { worker } from '../../../tests/mocks/worker';
import McpCompositeOauth from './McpCompositeOauth.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type VMCPResponseFixture = {
	id: string;
	displayName: string;
	created: string;
	components: Array<{
		id?: string;
		name?: string;
		mcpServerCatalogEntryID: string;
		catalogEntry: { manifest: { name: string; runtime: string } };
	}>;
};

function createVMCPResponse(id: string): VMCPResponseFixture {
	return {
		id,
		displayName: 'Virtual MCP',
		created: '2026-09-04T00:00:00Z',
		components: [
			{
				mcpServerCatalogEntryID: 'vmcp-component-entry',
				catalogEntry: {
					manifest: {
						name: 'Virtual MCP Component',
						runtime: 'remote'
					}
				}
			}
		]
	};
}

function mockConsentApis(
	pending: Array<Record<string, string>> = [],
	vmcp: ReturnType<typeof createVMCPResponse> = createVMCPResponse('')
) {
	const vmcpGet = vi.fn();
	const oauthPendingGet = vi.fn();

	worker.use(
		http.get('/api/vmcps/:id', ({ params }) => {
			const id = String(params.id);
			vmcpGet(id);
			return HttpResponse.json({ ...vmcp, id });
		}),
		http.get('/api/oauth/vmcp/:id', ({ params }) => {
			oauthPendingGet(String(params.id));
			return HttpResponse.json(pending);
		})
	);

	return { oauthPendingGet, vmcpGet };
}

describe('McpCompositeOauth', () => {
	it('loads migrated metadata by canonical ID while authenticating the legacy connection', async () => {
		const { oauthPendingGet, vmcpGet } = mockConsentApis();
		render(McpCompositeOauth, { compositeMcpId: 'ms1legacy', vmcpId: 'vmcp1migrated' });
		await expect.element(page.getByRole('heading', { name: 'Virtual MCP' })).toBeVisible();
		await expect.element(page.getByText('All services authenticated successfully!')).toBeVisible();
		await vi.waitFor(() => expect(oauthPendingGet).toHaveBeenCalledWith('ms1legacy'));
		expect(vmcpGet).toHaveBeenCalledWith('vmcp1migrated');
	});
	it('loads vMCP metadata from the vMCP endpoint for vmcp1 IDs', async () => {
		const id = 'vmcp1-consent-test';
		const icon = 'https://example.com/named-vmcp-component.svg';
		const { oauthPendingGet, vmcpGet } = mockConsentApis([
			{
				mcpServerID: 'vmcp-component-server',
				catalogEntryID: 'vmcp-component-entry',
				name: 'Named vMCP Component',
				icon,
				authURL: 'https://example.com/oauth'
			}
		]);

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText('Named vMCP Component', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('img', { name: 'icon', exact: true }))
			.toHaveAttribute('src', icon);
		await vi.waitFor(() => expect(vmcpGet).toHaveBeenCalledWith(id));
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('labels copied servers with their vMCP component names', async () => {
		const id = 'vmcp1-component-names';
		mockConsentApis(
			[
				{
					mcpServerID: 'server-b',
					catalogEntryID: 'gmail-entry',
					componentID: 'gmail-b',
					name: 'Gmail',
					authURL: 'https://example.com/b'
				},
				{
					mcpServerID: 'server-c',
					catalogEntryID: 'gmail-entry',
					componentID: 'gmail-c',
					name: 'Gmail',
					authURL: 'https://example.com/c'
				}
			],
			{
				id,
				displayName: 'Gmail, Gmail B, Gmail C',
				created: '2026-09-04T00:00:00Z',
				components: [
					{
						id: 'gmail-b',
						name: 'Gmail B',
						mcpServerCatalogEntryID: 'gmail-entry',
						catalogEntry: { manifest: { name: 'Gmail', runtime: 'remote' } }
					},
					{
						id: 'gmail-c',
						name: 'Gmail C',
						mcpServerCatalogEntryID: 'gmail-entry',
						catalogEntry: { manifest: { name: 'Gmail', runtime: 'remote' } }
					}
				]
			}
		);

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText('Gmail B', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Gmail C', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Gmail', { exact: true })).not.toBeInTheDocument();
	});

	it('falls back to the pending MCP server ID when its name is not set', async () => {
		const id = 'vmcp1-consent-fallback';
		const mcpServerID = 'component-server-without-name';
		const { oauthPendingGet, vmcpGet } = mockConsentApis([
			{
				mcpServerID,
				catalogEntryID: 'unknown-component-entry',
				authURL: 'https://example.com/oauth'
			}
		]);

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText(mcpServerID, { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('img', { name: 'icon', exact: true }))
			.not.toBeInTheDocument();
		await vi.waitFor(() => expect(vmcpGet).toHaveBeenCalledWith(id));
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('retries loading pending authentication when the page becomes visible', async () => {
		const id = 'vmcp1-pending-retry';
		let calls = 0;
		mockConsentApis();
		worker.use(
			http.get('/api/oauth/vmcp/:id', () => {
				calls += 1;
				if (calls === 1) {
					return HttpResponse.json({ message: 'Unable to load authentication' }, { status: 500 });
				}
				return HttpResponse.json([
					{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' }
				]);
			})
		);

		render(McpCompositeOauth, { compositeMcpId: id });
		await expect
			.element(page.getByText('Unable to load authentication', { exact: false }))
			.toBeVisible();

		document.dispatchEvent(new Event('visibilitychange'));

		await expect.element(page.getByText('GitHub', { exact: true })).toBeVisible();
		expect(calls).toBe(2);
	});

	it('checks only the launched service and shows row-local feedback', async () => {
		const id = 'vmcp1-row-check';
		const componentCheck = vi.fn();
		let finishCheck: () => void;
		const { oauthPendingGet } = mockConsentApis([
			{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' },
			{ mcpServerID: 'slack', name: 'Slack', authURL: 'https://example.com/slack' }
		]);
		worker.use(
			http.get('/api/oauth/vmcp/:id/components/:componentID', async ({ params }) => {
				componentCheck(String(params.componentID));
				await new Promise<void>((resolve) => (finishCheck = resolve));
				return HttpResponse.json({});
			})
		);

		render(McpCompositeOauth, { compositeMcpId: id });
		await page.getByRole('link', { name: 'Authenticate' }).first().click();
		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(() => expect(componentCheck).toHaveBeenCalledWith('github'));
		await expect.element(page.getByText('Checking for valid authentication…')).toBeVisible();
		await expect.element(page.getByText('Slack', { exact: true })).toBeVisible();
		expect(componentCheck).not.toHaveBeenCalledWith('slack');
		expect(oauthPendingGet).toHaveBeenCalledTimes(1);
		document.dispatchEvent(new Event('visibilitychange'));
		expect(componentCheck).toHaveBeenCalledTimes(1);
		finishCheck!();
		await expect.element(page.getByText('GitHub', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('Slack', { exact: true })).toBeVisible();
	});

	it('retains a failed row and lets the user retry', async () => {
		const id = 'vmcp1-row-retry';
		let calls = 0;
		const { oauthPendingGet } = mockConsentApis([
			{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' }
		]);
		worker.use(
			http.get('/api/oauth/vmcp/:id/components/:componentID', () => {
				calls += 1;
				if (calls === 1) return HttpResponse.json({ authURL: 'https://example.com/new' });
				if (calls === 2)
					return HttpResponse.json({ message: 'Authentication check failed' }, { status: 500 });
				return HttpResponse.json({});
			})
		);

		const onComplete = vi.fn();
		render(McpCompositeOauth, { compositeMcpId: id, onComplete });
		await page.getByRole('link', { name: 'Authenticate' }).click();
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.waitFor(() => expect(calls).toBe(1));
		await expect
			.element(page.getByRole('link', { name: 'Authenticate' }))
			.toHaveAttribute('href', 'https://example.com/new');

		await page.getByRole('link', { name: 'Authenticate' }).click();
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.waitFor(() => expect(calls).toBe(2));
		await expect
			.element(page.getByText('Authentication check failed', { exact: false }))
			.toBeVisible();

		await page.getByRole('button', { name: 'Retry' }).click();
		await vi.waitFor(() => expect(calls).toBe(3));

		await vi.waitFor(() => expect(onComplete).toHaveBeenCalledTimes(1));
		expect(oauthPendingGet).toHaveBeenCalledTimes(1);
	});
});

it('keeps mixed client authentication pending after a local component completes', async () => {
	const checked = vi.fn();
	mockConsentApis([
		{
			mcpServerID: 'local',
			name: 'Local',
			authURL: `${window.location.origin}/oauth/mcp/login/ui-local-component`
		},
		{ mcpServerID: 'hosted', name: 'Hosted', authURL: 'https://provider.example/authorize' }
	]);
	worker.use(
		http.get('/api/oauth/vmcp/:id/components/:componentID', ({ request, params }) => {
			checked(params.componentID, new URL(request.url).searchParams.get('oauth_auth_request'));
			return HttpResponse.json({ authURL: '' });
		})
	);
	const onComplete = vi.fn();
	await render(McpCompositeOauth, {
		compositeMcpId: 'vmcp1mixed',
		oauthAuthRequestId: 'original-request',
		onComplete
	});
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login "${window.location.origin}/oauth/mcp/login/ui-local-component"`
		);
	worker.use(
		http.get('*/oauth/mcp/login/ui-local-component', () => new HttpResponse(null, { status: 400 }))
	);
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(checked).toHaveBeenCalledWith('local', 'original-request'));
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.toHaveAttribute('href', 'https://provider.example/authorize');
	await expect
		.element(page.getByText('All services authenticated successfully!'))
		.not.toBeInTheDocument();
	expect(onComplete).not.toHaveBeenCalled();
});

it('embeds login instructions without duplicating the parent dialog heading', async () => {
	mockConsentApis([
		{
			mcpServerID: 'vercel',
			name: 'Vercel',
			authURL: '/oauth/mcp/login/ui-local-vercel'
		}
	]);
	render(McpCompositeOauth, { compositeMcpId: 'vmcp1-embedded', embedded: true });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect.element(page.getByText('Vercel', { exact: true })).toBeVisible();
	await expect.element(page.getByLabelText('Authentication command')).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Generate new command', exact: true }))
		.toBeVisible();
	await expect.element(page.getByRole('heading', { name: 'Virtual MCP' })).not.toBeInTheDocument();
});

it.each([390, 1280])('keeps mixed authentication readable at %ipx', async (width) => {
	await page.viewport(width, 900);
	try {
		mockConsentApis([
			{ mcpServerID: 'vercel', name: 'Vercel', authURL: '/oauth/mcp/login/ui-local-vercel' },
			{
				mcpServerID: 'calendar',
				name: 'Microsoft 365 Calendar',
				authURL: 'https://provider.example/calendar'
			},
			{
				mcpServerID: 'second-local',
				name: 'Another locally authenticated MCP server',
				authURL: '/oauth/mcp/login/ui-local-second'
			}
		]);
		render(McpCompositeOauth, {
			compositeMcpId: 'vmcp1-layout',
			embedded: true,
			class: 'max-w-lg'
		});
		await page
			.getByRole('group', { name: 'Vercel', exact: true })
			.getByRole('button', { name: 'Authenticate', exact: true })
			.click();
		await page
			.getByRole('group', { name: 'Another locally authenticated MCP server', exact: true })
			.getByRole('button', { name: 'Authenticate', exact: true })
			.click();
		const commands = page.getByLabelText('Authentication command');
		await expect.element(commands.nth(1)).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
			.toHaveAttribute('href', 'https://provider.example/calendar');
		const name = page.getByText('Vercel', { exact: true }).element().getBoundingClientRect();
		const command = commands.first().element().getBoundingClientRect();
		expect(command.top).toBeGreaterThan(name.bottom);
		expect(command.width).toBeGreaterThan(250);
		expect(command.right).toBeLessThanOrEqual(width);
	} finally {
		await page.viewport(1280, 720);
	}
});

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});

it('automatically resumes only after both local login and browser OAuth have finished', async () => {
	let consumed = false;
	const onComplete = vi.fn();
	const checked = vi.fn();
	mockConsentApis([
		{
			mcpServerID: 'vercel',
			name: 'Vercel',
			authURL: `${window.location.origin}/oauth/mcp/login/mixed-completion`
		},
		{ mcpServerID: 'outlook', name: 'Outlook', authURL: 'https://provider.example/authorize' }
	]);
	worker.use(
		http.get('*/oauth/mcp/login/mixed-completion', () =>
			consumed ? new HttpResponse(null, { status: 400 }) : HttpResponse.json({ expiresIn: 600 })
		),
		http.get('/api/oauth/vmcp/:id/components/:componentID', ({ params }) => {
			checked(params.componentID);
			return HttpResponse.json({ authURL: '' });
		})
	);
	await render(McpCompositeOauth, { compositeMcpId: 'vmcp1mixed-completion', onComplete });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect.element(page.getByLabelText('Authentication command')).toBeVisible();
	await expect.element(page.getByRole('timer')).toHaveTextContent('Command expires in 10:00');
	consumed = true;
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
	expect(checked).toHaveBeenCalledWith('vercel');
	expect(checked).not.toHaveBeenCalledWith('outlook');
	expect(onComplete).not.toHaveBeenCalled();
	await page.getByRole('link', { name: 'Authenticate', exact: true }).click();
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	expect(checked).toHaveBeenCalledWith('outlook');
	await expect.element(page.getByText('All services authenticated successfully!')).toBeVisible();
});

it('automatically retries a failed component check after local login is consumed', async () => {
	mockConsentApis([
		{ mcpServerID: 'local-retry', name: 'Local Retry', authURL: '/oauth/mcp/login/component-retry' }
	]);
	let checks = 0;
	const onComplete = vi.fn();
	worker.use(
		http.get('*/oauth/mcp/login/component-retry', () => new HttpResponse(null, { status: 400 })),
		http.get('/api/oauth/vmcp/:id/components/:componentID', () => {
			checks++;
			return checks === 1
				? HttpResponse.json({ message: 'Temporary authentication check failure' }, { status: 503 })
				: HttpResponse.json({ authURL: '' });
		})
	);
	await render(McpCompositeOauth, { compositeMcpId: 'vmcp1-retry', onComplete });
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	expect(checks).toBe(2);
	await expect.element(page.getByText('All services authenticated successfully!')).toBeVisible();
});
