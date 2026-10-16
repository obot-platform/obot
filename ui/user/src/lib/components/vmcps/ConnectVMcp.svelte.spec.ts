import { page as appPage } from '$app/state';
import { UserService, type VMCP, type VMCPConfiguration, type VMCPInstance } from '$lib/services';
import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
import { vmcpInstanceNeedsUserConfiguration } from '$lib/services/vmcps/utils';
import { vmcpInstances } from '$lib/stores';
import { goto, replaceState } from '$lib/url';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import ConnectVMcp from './ConnectVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn().mockResolvedValue(undefined),
	replaceState: vi.fn()
}));

function expectedOAuthPath(entryID: string, returnParam?: { key: string; value: string }) {
	const params = new URLSearchParams({ 'configure-oauth': 'true' });
	if (returnParam) {
		const returnURL = new URL(appPage.url);
		returnURL.searchParams.set(returnParam.key, returnParam.value);
		params.set('oauth-redirect', `${returnURL.pathname}${returnURL.search}${returnURL.hash}`);
	}
	return `/mcp-servers/c/${encodeURIComponent(entryID)}?${params.toString()}`;
}

function configurableVMcp(): VMCP {
	const vmcp = createVMCP({ id: 'vmcp1configurable', displayName: 'Configured vMCP' });
	vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
	vmcp.components![0].catalogEntry.manifest.config = [
		{
			key: 'API_TOKEN',
			name: 'API token',
			description: 'Token used by this server',
			required: true,
			sensitive: true,
			value: '',
			usage: 'env'
		}
	];
	return vmcp;
}

async function renderDialog(vmcp: VMCP, instance?: VMCPInstance, options?: VMcpConnectOptions) {
	await preparePageData();
	await vmcpInstances.refresh();
	const result = await render(ConnectVMcp, { reconciliationRetryMs: 100 });
	result.component.open(vmcp, instance, options);
	return result;
}

function introSetupText() {
	return page.getByText('This will begin the initial setup process for this server.', {
		exact: false
	});
}

async function continueFromIntro() {
	await expect.element(introSetupText()).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();
}

describe('ConnectVMcp.svelte', () => {
	beforeEach(() => {
		vi.mocked(goto).mockClear();
		vi.mocked(replaceState).mockClear();
		vmcpInstances.current = { items: [], loading: false };
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [] })),
			http.post('/api/vmcp-instances/:id/reveal', () => HttpResponse.json({ components: {} })),
			http.get('/api/vmcps/:id/oauth-url', () => HttpResponse.json({ oauthURL: '' }))
		);
	});

	it('opens the inspector when Test is clicked for an existing instance', async () => {
		const vmcp = createVMCP({ id: 'vmcp1configured', displayName: 'Configured vMCP' });
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};

		await renderDialog(vmcp, existing);
		await page.getByRole('button', { name: 'Test vMCP' }).click();

		expect(vi.mocked(goto)).toHaveBeenCalledWith(`/vmcps/${vmcp.id}?view=inspector`);
		await expect.element(introSetupText()).not.toBeVisible();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();
	});

	function createdInstance(vmcp: VMCP, status?: VMCPInstance['status']): VMCPInstance {
		return {
			id: 'vmcpi-created',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status
		};
	}

	it.each(['none', 'deleted', 'other user', 'other vMCP'])(
		'does not check OAuth status when the only connection is %s',
		async (connection) => {
			const vmcp = configurableVMcp();
			const candidate = createdInstance(vmcp, { configured: true });
			if (connection === 'deleted') candidate.deleted = '2026-01-02T00:00:00Z';
			if (connection === 'other user') candidate.userID = 'another-user';
			if (connection === 'other vMCP') candidate.vmcpID = 'another-vmcp';
			worker.use(
				http.get('/api/vmcp-instances', () =>
					HttpResponse.json({ items: connection === 'none' ? [] : [candidate] })
				)
			);
			const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
			try {
				await renderDialog(vmcp);
				await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
				expect(status).not.toHaveBeenCalled();
			} finally {
				status.mockRestore();
			}
		}
	);

	it.each([
		{ source: 'argument', configured: true },
		{ source: 'argument', configured: false },
		{ source: 'store', configured: true },
		{ source: 'store', configured: false }
	])(
		'checks OAuth status for an existing $source connection (configured: $configured)',
		async ({ source, configured }) => {
			const vmcp = configurableVMcp();
			const existing = createdInstance(vmcp, { configured });
			const status = vi.fn();
			worker.use(
				http.get('/api/vmcp-instances', () =>
					HttpResponse.json({ items: source === 'store' ? [existing] : [] })
				),
				http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
					status();
					return HttpResponse.json({ authenticatedComponents: [] });
				})
			);

			await renderDialog(vmcp, source === 'argument' ? existing : undefined);
			await vi.waitFor(() => expect(status).toHaveBeenCalledOnce());
		}
	);

	it('opens the inspector only after the instance reports configured', async () => {
		const vmcp = createVMCP({ id: 'vmcp1delayed', displayName: 'Plain vMCP' });
		mockConfigureAndLaunch(vmcp);
		let reads = 0;
		let finishReconciliation!: () => void;
		const reconciliation = new Promise<void>((resolve) => {
			finishReconciliation = resolve;
		});
		worker.use(
			http.get('/api/vmcp-instances/vmcpi-created', async () => {
				reads += 1;
				if (reads >= 3) await reconciliation;
				return HttpResponse.json(
					createdInstance(
						vmcp,
						reads >= 3 ? { configured: true, configurationCheckHash: 'checked' } : undefined
					)
				);
			})
		);

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Test vMCP' }).click();
		await continueFromIntro();

		await vi.waitFor(() => expect(reads).toBe(3));
		expect(vi.mocked(goto)).not.toHaveBeenCalled();
		finishReconciliation();

		await vi.waitFor(
			() => {
				expect(vi.mocked(goto)).toHaveBeenCalledWith(`/vmcps/${vmcp.id}?view=inspector`);
			},
			{ timeout: 4000 }
		);
		expect(reads).toBe(4);
		expect(vmcpInstances.current.items[0]?.status?.configured).toBe(true);
	}, 8000);

	it('retries when the configuration check reports an incomplete instance', async () => {
		const vmcp = createVMCP({ id: 'vmcp1unconfigured', displayName: 'Plain vMCP' });
		mockConfigureAndLaunch(vmcp);
		const getInstance = vi.fn();
		worker.use(
			http.get('/api/vmcp-instances/vmcpi-created', () => {
				getInstance();
				return HttpResponse.json(
					createdInstance(vmcp, {
						configured: false,
						configurationCheckHash: 'checked'
					})
				);
			})
		);

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Test vMCP' }).click();
		await continueFromIntro();

		await vi.waitFor(
			() => {
				expect(vi.mocked(goto)).toHaveBeenCalledWith(`/vmcps/${vmcp.id}?view=inspector`);
			},
			{ timeout: 7000 }
		);
		expect(getInstance).toHaveBeenCalledTimes(6);
	}, 9000);

	it('starts setup from Test and opens the inspector after launch when there is no instance', async () => {
		const vmcp = createVMCP({ id: 'vmcp1testlaunch', displayName: 'Plain vMCP' });
		const { launch } = mockConfigureAndLaunch(vmcp);

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Test vMCP' }).click();
		await continueFromIntro();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		await vi.waitFor(
			() => {
				expect(vi.mocked(goto)).toHaveBeenCalledWith(`/vmcps/${vmcp.id}?view=inspector`);
			},
			{ timeout: 3000 }
		);
	}, 5000);

	it('opens the connect dialog and starts setup from Preconfigure when there is no instance', async () => {
		await renderDialog(configurableVMcp());
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		await expect.element(introSetupText()).not.toBeVisible();
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await expect.element(introSetupText()).toBeVisible();
		await expect
			.element(
				page.getByText('Additional configuration details may also be required', { exact: false })
			)
			.toBeVisible();
	});

	function salesforceOAuthVMcp() {
		const vmcp = createVMCP({ id: 'vmcp1salesforce', displayName: 'Salesforce vMCP' });
		vmcp.components![0].name = 'Salesforce';
		vmcp.components![0].mcpServerCatalogEntryID = 'salesforce';
		vmcp.components![0].catalogEntry.manifest.runtime = 'remote';
		vmcp.components![0].catalogEntry.manifest.remoteConfig = {
			fixedURL: 'https://api.salesforce.com/platform/mcp/v1/platform/sobject-all',
			staticOAuthRequired: true
		};
		vmcp.status = {
			ready: false,
			components: [{ name: 'Salesforce', error: 'static OAuth credentials are not configured' }]
		};
		return vmcp;
	}

	it('guides setup without launching when a component lacks static OAuth credentials', async () => {
		const vmcp = salesforceOAuthVMcp();
		const { createInstance, launch } = mockConfigureAndLaunch(vmcp);

		await renderDialog(vmcp);
		const connectDialog = page.getByCSS('#connect-to-vmcp-dialog');
		await expect.element(page.getByLabelText('Connection URL')).not.toBeInTheDocument();
		await expect
			.element(
				connectDialog.getByText('Salesforce requires administrator OAuth setup.', { exact: false })
			)
			.toBeVisible();
		await expect
			.element(connectDialog.getByRole('button', { name: 'Preconfigure server' }))
			.not.toBeInTheDocument();
		await expect
			.element(connectDialog.getByRole('link', { name: 'Configure Salesforce OAuth' }))
			.toHaveAttribute('href', '/mcp-servers/c/salesforce?configure-oauth=true');
		await expect
			.element(connectDialog.getByRole('link', { name: 'Configure Salesforce OAuth' }))
			.toHaveClass('btn btn-primary');
		expect(createInstance).not.toHaveBeenCalled();
		expect(launch).not.toHaveBeenCalled();
	});

	it('stores connect=<vmcp id> before opening OAuth setup from the list', async () => {
		const vmcp = salesforceOAuthVMcp();
		await renderDialog(vmcp, undefined, { connectReturn: 'list' });
		const connectDialog = page.getByCSS('#connect-to-vmcp-dialog');

		const link = connectDialog.getByRole('link', { name: 'Configure Salesforce OAuth' });
		await expect
			.element(link)
			.toHaveAttribute('href', expectedOAuthPath('salesforce', { key: 'connect', value: vmcp.id }));
		await link.click();

		const url = vi.mocked(replaceState).mock.calls[0]?.[0] as URL;
		expect(url.searchParams.get('connect')).toBe(vmcp.id);
		expect(vi.mocked(goto)).toHaveBeenCalledWith(
			expectedOAuthPath('salesforce', { key: 'connect', value: vmcp.id })
		);
	});

	it('stores connect=true before opening OAuth setup from the designer', async () => {
		const vmcp = salesforceOAuthVMcp();
		await renderDialog(vmcp, undefined, { connectReturn: 'designer' });
		const connectDialog = page.getByCSS('#connect-to-vmcp-dialog');

		await connectDialog.getByRole('link', { name: 'Configure Salesforce OAuth' }).click();

		const url = vi.mocked(replaceState).mock.calls[0]?.[0] as URL;
		expect(url.searchParams.get('connect')).toBe('true');
		expect(vi.mocked(goto)).toHaveBeenCalledWith(
			expectedOAuthPath('salesforce', { key: 'connect', value: 'true' })
		);
	});

	it('stores inspector=<vmcp id> and a return path before opening OAuth setup', async () => {
		const vmcp = salesforceOAuthVMcp();
		await renderDialog(vmcp, undefined, { connectReturn: 'inspector' });
		const connectDialog = page.getByCSS('#connect-to-vmcp-dialog');

		await connectDialog.getByRole('link', { name: 'Configure Salesforce OAuth' }).click();

		const url = vi.mocked(replaceState).mock.calls[0]?.[0] as URL;
		expect(url.searchParams.get('inspector')).toBe(vmcp.id);
		expect(vi.mocked(goto)).toHaveBeenCalledWith(
			expectedOAuthPath('salesforce', { key: 'inspector', value: vmcp.id })
		);
	});

	function mockConfigureAndLaunch(
		vmcp: VMCP,
		opts?: {
			launchError?: string;
			launchDelayMs?: number;
			oauthURL?: string | ((check: number) => string);
		}
	) {
		const createInstance = vi.fn();
		const configureInstance = vi.fn();
		const launch = vi.fn();
		let oauthChecks = 0;
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [] })),
			http.post('/api/vmcp-instances', async ({ request }) => {
				createInstance(await request.json());
				return HttpResponse.json({
					id: 'vmcpi-created',
					vmcpID: vmcp.id,
					userID: getProfileResponse.id,
					created: '2026-01-01T00:00:00Z'
				});
			}),
			http.post('/api/vmcp-instances/vmcpi-created/configure', async ({ request }) => {
				configureInstance(await request.json());
				return HttpResponse.json({
					id: 'vmcpi-created',
					vmcpID: vmcp.id,
					userID: getProfileResponse.id,
					created: '2026-01-01T00:00:00Z'
				});
			}),
			http.get('/api/vmcp-instances/vmcpi-created', () =>
				HttpResponse.json({
					id: 'vmcpi-created',
					vmcpID: vmcp.id,
					userID: getProfileResponse.id,
					created: '2026-01-01T00:00:00Z',
					status: { configured: true, configurationCheckHash: 'checked' }
				})
			),
			http.post(`/api/vmcps/${vmcp.id}/launch`, async () => {
				launch();
				if (opts?.launchDelayMs) {
					await new Promise((resolve) => setTimeout(resolve, opts.launchDelayMs));
				}
				if (opts?.launchError) {
					return HttpResponse.text(opts.launchError, { status: 503 });
				}
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () => {
				oauthChecks += 1;
				const oauthURL =
					typeof opts?.oauthURL === 'function'
						? opts.oauthURL(oauthChecks)
						: (opts?.oauthURL ?? '');
				return HttpResponse.json({ oauthURL });
			})
		);
		return { createInstance, configureInstance, launch, oauthChecks: () => oauthChecks };
	}

	async function configureFromPreconfigure() {
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();
		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await tokenField.click();
		await tokenField.fill('secret-token');
		await page.getByRole('button', { name: 'Configure', exact: true }).click();
	}

	it('launches immediately from Preconfigure when there is no user configuration', async () => {
		const vmcp = createVMCP({ id: 'vmcp1noconfig', displayName: 'Plain vMCP' });
		const { launch, createInstance, configureInstance } = mockConfigureAndLaunch(vmcp, {
			launchDelayMs: 80
		});

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();

		await expect.element(page.getByText('Launching vMCP...')).toBeVisible();
		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(createInstance).toHaveBeenCalledWith({ vmcpID: vmcp.id });
		expect(configureInstance).not.toHaveBeenCalled();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
	});

	it('creates and configures an instance from Preconfigure', async () => {
		const vmcp = configurableVMcp();
		const { createInstance, configureInstance, launch } = mockConfigureAndLaunch(vmcp);

		await renderDialog(vmcp);
		await configureFromPreconfigure();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(createInstance).toHaveBeenCalledWith({ vmcpID: vmcp.id });
		expect(configureInstance).toHaveBeenCalledWith({
			components: { 'component-entry-default': { API_TOKEN: 'secret-token' } }
		} satisfies VMCPConfiguration);
		await expect
			.element(page.getByText('This server has already been configured.', { exact: false }))
			.toBeVisible();
	});

	it('refreshes the pending instance after launch before reopening the connect dialog', async () => {
		const vmcp = configurableVMcp();
		const { launch } = mockConfigureAndLaunch(vmcp);
		let reconciled = false;
		let reads = 0;
		worker.use(
			http.get('/api/vmcp-instances/vmcpi-created', () => {
				reads += 1;
				return HttpResponse.json(
					createdInstance(
						vmcp,
						reconciled ? { configured: true, configurationCheckHash: 'checked' } : undefined
					)
				);
			}),
			http.post(`/api/vmcps/${vmcp.id}/launch`, () => {
				launch();
				reconciled = true;
				return HttpResponse.json({});
			})
		);

		const result = await renderDialog(vmcp);
		await configureFromPreconfigure();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce(), { timeout: 5000 });
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		expect(reads).toBeGreaterThan(5);
		expect(vmcpInstances.current.items[0]?.status?.configured).toBe(true);

		result.component.open(vmcp);
		await page.getByRole('button', { name: 'Test vMCP' }).click();
		expect(vi.mocked(goto)).toHaveBeenCalledWith(`/vmcps/${vmcp.id}?view=inspector`);
		expect(launch).toHaveBeenCalledOnce();
		await expect.element(introSetupText()).not.toBeVisible();
	}, 8000);

	it('shows launch progress and an error when launch fails', async () => {
		const vmcp = configurableVMcp();
		const { configureInstance, launch } = mockConfigureAndLaunch(vmcp, {
			launchError: 'Component MCP server is not healthy'
		});

		await renderDialog(vmcp);
		await configureFromPreconfigure();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(configureInstance).toHaveBeenCalledOnce();
		await expect.element(page.getByText('vMCP Launch Failed')).toBeVisible();
		await expect
			.element(page.getByText('Component MCP server is not healthy', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Update Configuration and Try Again' }))
			.toBeVisible();
	});

	it('shows configured state and allows editing an existing instance', async () => {
		const vmcp = configurableVMcp();
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		worker.use(
			http.get('/api/vmcp-instances', () =>
				HttpResponse.json({
					items: [existing]
				})
			)
		);

		await renderDialog(vmcp, existing);
		await expect.element(introSetupText()).not.toBeVisible();
		await expect
			.element(page.getByText('This server has already been configured.', { exact: false }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Edit configuration' }).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	});

	it('opens instance configuration from the latest component fields', async () => {
		const vmcp = configurableVMcp();
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-entry-default.API_TOKEN']
			}
		};
		const revealed = vi.fn();
		worker.use(
			http.post('/api/vmcp-instances/vmcpi-existing/reveal', () => {
				revealed();
				return HttpResponse.json({
					components: { 'component-entry-default': { API_TOKEN: 'saved-token' } }
				});
			})
		);

		await preparePageData();
		await vmcpInstances.refresh();
		const result = await render(ConnectVMcp, { reconciliationRetryMs: 100 });
		await result.component.openEditConfiguration(vmcp, existing);

		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
		await vi.waitFor(() => expect(revealed).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible();
	});

	it('refreshes instance status after editing configuration when configure returns stale missing fields', async () => {
		const vmcp = configurableVMcp();
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-entry-default.API_TOKEN']
			}
		};
		const getInstance = vi.fn();
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
			http.post('/api/vmcp-instances/vmcpi-existing/reveal', () =>
				HttpResponse.json({
					components: { 'component-entry-default': { API_TOKEN: 'saved-token' } }
				})
			),
			http.post('/api/vmcp-instances/vmcpi-existing/configure', () => HttpResponse.json(existing)),
			http.get('/api/vmcp-instances/vmcpi-existing', () => {
				getInstance();
				return HttpResponse.json({
					...existing,
					status: { configured: true, configurationCheckHash: 'checked' }
				});
			}),
			http.post(`/api/vmcps/${vmcp.id}/launch`, () => HttpResponse.json({})),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () => HttpResponse.json({ oauthURL: '' }))
		);

		await preparePageData();
		vmcpInstances.current = { items: [existing], loading: false };
		const result = await render(ConnectVMcp, { reconciliationRetryMs: 100 });
		await result.component.openEditConfiguration(vmcp, existing);

		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await tokenField.click();
		await tokenField.fill('secret-token');
		await page.getByRole('button', { name: 'Update', exact: true }).click();

		await vi.waitFor(() => {
			expect(vmcpInstanceNeedsUserConfiguration(vmcpInstances.current.items[0])).toBe(false);
			expect(getInstance).toHaveBeenCalled();
		});
	});

	it('prompts for OAuth after launch and continues when authentication completes', async () => {
		const vmcp = createVMCP({ id: 'vmcp1oauth', displayName: 'OAuth vMCP' });
		const { launch, oauthChecks } = mockConfigureAndLaunch(vmcp, {
			oauthURL: (check) => (check === 1 ? 'https://auth.example.com/authorize' : '')
		});

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();

		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(() => {
			expect(oauthChecks()).toBe(2);
		});
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
	});

	it('keeps onConnected until OAuth completes after the configure dialog close animation', async () => {
		const vmcp = createVMCP({ id: 'vmcp1oauth-inspector', displayName: 'OAuth vMCP' });
		const onConnected = vi.fn();
		const onDismissed = vi.fn();
		const { launch, oauthChecks } = mockConfigureAndLaunch(vmcp, {
			oauthURL: (check) => (check === 1 ? 'https://auth.example.com/authorize' : '')
		});

		await renderDialog(vmcp, undefined, { onConnected, onDismissed });
		await continueFromIntro();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await new Promise((resolve) => setTimeout(resolve, 250));
		expect(onDismissed).not.toHaveBeenCalled();
		expect(onConnected).not.toHaveBeenCalled();

		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(() => {
			expect(oauthChecks()).toBe(2);
			expect(onConnected).toHaveBeenCalledOnce();
		});
		expect(onDismissed).not.toHaveBeenCalled();
	}, 4000);

	it('reauthenticates the only authenticated component without the picker', async () => {
		const vmcp = createVMCP({ id: 'vmcp1reauth', displayName: 'Remote vMCP' }, [
			createMCPCatalogEntry({ id: 'entry-remote', name: 'Remote', runtime: 'remote' })
		]);
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		const clearedAll = vi.fn();
		const clearedComponent = vi.fn();
		let statusReads = 0;
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				statusReads += 1;
				return HttpResponse.json({
					authenticatedComponents: statusReads === 1 ? ['component-entry-remote'] : []
				});
			}),
			http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => {
				clearedAll();
				return HttpResponse.json({});
			}),
			http.delete(`/api/vmcps/${vmcp.id}/components/:componentID/oauth`, ({ params }) => {
				clearedComponent(params.componentID);
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			)
		);

		await renderDialog(vmcp, existing);
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Edit configuration' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Reauthenticate', exact: true }).click();

		await vi.waitFor(() =>
			expect(clearedComponent).toHaveBeenCalledExactlyOnceWith('component-entry-remote')
		);
		expect(clearedAll).not.toHaveBeenCalled();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate All' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await vi.waitFor(() => expect(statusReads).toBeGreaterThan(1));
	});

	it('clears OAuth and prompts to authenticate from Reauthenticate All', async () => {
		const vmcp = createVMCP({ id: 'vmcp1reauthall', displayName: 'Remote vMCP' }, [
			createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' }),
			createMCPCatalogEntry({ id: 'entry-brave', name: 'Brave Search', runtime: 'remote' })
		]);
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		const cleared = vi.fn();
		let statusReads = 0;
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				statusReads += 1;
				return HttpResponse.json({
					authenticatedComponents:
						statusReads === 1 ? ['component-entry-outlook', 'component-entry-brave'] : []
				});
			}),
			http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => {
				cleared();
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			)
		);

		await renderDialog(vmcp, existing);
		await page.getByRole('button', { name: 'Reauthenticate', exact: true }).click();
		await expect.element(page.getByRole('button', { name: 'Reauthenticate All' })).toBeVisible();
		await page.getByRole('button', { name: 'Reauthenticate All' }).click();

		await vi.waitFor(() => expect(cleared).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await vi.waitFor(() => expect(statusReads).toBeGreaterThan(1));
	});

	it.each(['completion', 'close button', 'Escape', 'no OAuth required'])(
		'returns to the connect dialog after single-server reauthentication: %s',
		async (exit) => {
			const vmcp = createVMCP({ id: 'vmcp1reauthpick', displayName: 'Remote vMCP' }, [
				createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' }),
				createMCPCatalogEntry({ id: 'entry-brave', name: 'Brave Search', runtime: 'remote' })
			]);
			const existing: VMCPInstance = {
				id: 'vmcpi-existing',
				vmcpID: vmcp.id,
				userID: getProfileResponse.id,
				created: '2026-01-01T00:00:00Z',
				status: { configured: true }
			};
			const clearedAll = vi.fn();
			const clearedComponent = vi.fn();
			let statusReads = 0;
			let authenticated = ['component-entry-outlook', 'component-entry-brave'];
			let oauthChecks = 0;
			worker.use(
				http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
				http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
					statusReads += 1;
					return HttpResponse.json({ authenticatedComponents: authenticated });
				}),
				http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => {
					clearedAll();
					return HttpResponse.json({});
				}),
				http.delete(`/api/vmcps/${vmcp.id}/components/:componentID/oauth`, ({ params }) => {
					clearedComponent(params.componentID);
					authenticated = authenticated.filter((id) => id !== params.componentID);
					return HttpResponse.json({});
				}),
				http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () => {
					oauthChecks += 1;
					return HttpResponse.json({
						oauthURL:
							exit !== 'no OAuth required' && oauthChecks === 1
								? 'https://auth.example.com/authorize'
								: ''
					});
				})
			);

			await renderDialog(vmcp, existing);
			await page.getByRole('button', { name: 'Reauthenticate', exact: true }).click();
			await expect
				.element(page.getByRole('button', { name: 'Reauthenticate Outlook' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Reauthenticate Brave Search' }))
				.toBeVisible();
			await page.getByRole('button', { name: 'Reauthenticate Outlook' }).click();

			await vi.waitFor(() =>
				expect(clearedComponent).toHaveBeenCalledExactlyOnceWith('component-entry-outlook')
			);
			expect(clearedAll).not.toHaveBeenCalled();
			if (exit !== 'no OAuth required') {
				await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
				await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();
				// Let the connect dialog's native close event finish before completing OAuth.
				await new Promise((resolve) => setTimeout(resolve, 250));
				if (exit === 'completion') document.dispatchEvent(new Event('visibilitychange'));
				else if (exit === 'Escape') await userEvent.keyboard('{Escape}');
				else await page.getByRole('button', { name: 'Close dialog' }).click();
			}
			await vi.waitFor(() => expect(statusReads).toBeGreaterThan(1));
			await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
			await expect.element(page.getByText('Connection URL', { exact: true })).toBeVisible();
			expect(authenticated).toEqual(['component-entry-brave']);
		}
	);

	it.each([
		{ selection: 'All', pendingStep: 'clear' },
		{ selection: 'Outlook', pendingStep: 'clear' },
		{ selection: 'All', pendingStep: 'oauth-url' },
		{ selection: 'Outlook', pendingStep: 'oauth-url' }
	])(
		'ignores superseded reauthentication of $selection during $pendingStep',
		async ({ selection, pendingStep }) => {
			const vmcp = createVMCP({ id: `vmcp1stale-${selection}-${pendingStep}` }, [
				createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' }),
				createMCPCatalogEntry({ id: 'entry-brave', name: 'Brave Search', runtime: 'remote' })
			]);
			let releaseOld!: () => void;
			const oldRequest = new Promise<void>((resolve) => (releaseOld = resolve));
			let releaseStatus!: () => void;
			const newStatus = new Promise<void>((resolve) => (releaseStatus = resolve));
			const pendingStarted = vi.fn();
			const clear = async () => {
				if (pendingStep === 'clear') {
					pendingStarted();
					await oldRequest;
				}
				return HttpResponse.json({});
			};
			worker.use(
				http.delete(`/api/vmcps/${vmcp.id}/oauth`, clear),
				http.delete(`/api/vmcps/${vmcp.id}/components/:componentID/oauth`, clear),
				http.get(`/api/vmcps/${vmcp.id}/oauth-url`, async () => {
					if (pendingStep === 'oauth-url') {
						pendingStarted();
						await oldRequest;
					}
					return HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' });
				}),
				http.get(`/api/vmcps/${vmcp.id}/oauth-status`, async () => {
					await newStatus;
					return HttpResponse.json({ authenticatedComponents: [] });
				})
			);
			const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
			try {
				await preparePageData();
				const { component } = await render(ConnectVMcp);
				component.openReauthenticate(vmcp, ['component-entry-outlook', 'component-entry-brave']);
				await page
					.getByRole('button', { name: `Reauthenticate ${selection}`, exact: true })
					.click();
				await vi.waitFor(() => expect(pendingStarted).toHaveBeenCalledOnce());
				await userEvent.keyboard('{Escape}');
				// Reopening the same vMCP must also supersede the earlier operation.
				component.open(vmcp, createdInstance(vmcp, { configured: true }));
				expect(status).toHaveBeenCalledOnce();
				const signal = status.mock.calls[0][1]?.signal;
				releaseOld();
				// Allow the old response and picker close event to finish their continuations.
				await new Promise((resolve) => setTimeout(resolve, 250));
				expect(status).toHaveBeenCalledOnce();
				expect(signal?.aborted).toBe(false);
				await expect
					.element(page.getByRole('link', { name: 'Authenticate' }))
					.not.toBeInTheDocument();
				await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
			} finally {
				releaseOld();
				releaseStatus();
				status.mockRestore();
			}
		}
	);

	it('cancels the previous OAuth status request when another starts', async () => {
		const vmcp = configurableVMcp();
		vmcp.id = 'vmcp1cancelstatus';
		const existing = createdInstance(vmcp, { configured: true });
		let release!: () => void;
		const pending = new Promise<void>((resolve) => (release = resolve));
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, async () => {
				await pending;
				return HttpResponse.json({ authenticatedComponents: [] });
			})
		);
		const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
		try {
			const { component } = await renderDialog(vmcp, existing);
			const firstSignal = status.mock.calls[0][1]?.signal;
			expect(firstSignal?.aborted).toBe(false);
			component.open(vmcp, existing);
			expect(firstSignal?.aborted).toBe(true);
			expect(status.mock.calls[1][1]?.signal?.aborted).toBe(false);
		} finally {
			release();
			status.mockRestore();
		}
	});

	it('does not finish a newer flow after cancelling an OAuth completion status request', async () => {
		const vmcp = configurableVMcp();
		vmcp.id = 'vmcp1cancelcompletion';
		let release!: () => void;
		const pending = new Promise<void>((resolve) => (release = resolve));
		const statusStarted = vi.fn();
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			),
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, async () => {
				statusStarted();
				await pending;
				return HttpResponse.json({ authenticatedComponents: [] });
			})
		);
		try {
			const { component } = await renderDialog(vmcp);
			await component.authenticate();
			await page.getByRole('button', { name: 'Close dialog' }).click();
			await vi.waitFor(() => expect(statusStarted).toHaveBeenCalledOnce());
			const onConnected = vi.fn();
			component.open(createVMCP({ id: 'vmcp1newflow' }), undefined, { onConnected });
			release();
			await expect.element(introSetupText()).toBeVisible();
			// Allow the previous dialog's close event and aborted fetch continuation to settle.
			await new Promise((resolve) => setTimeout(resolve, 250));
			expect(onConnected).not.toHaveBeenCalled();
			await component.authenticate();
			expect(onConnected).toHaveBeenCalledOnce();
		} finally {
			release();
		}
	});

	it('returns to connection instructions after editing then reauthenticating all', async () => {
		const vmcp = configurableVMcp();
		const extra = createVMCP({}, [
			createMCPCatalogEntry({ id: 'entry-extra', name: 'Extra', runtime: 'remote' })
		]);
		vmcp.components!.push(...extra.components!);
		const existing = createdInstance(vmcp, { configured: true });
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}`, () => HttpResponse.json(vmcp)),
			http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => HttpResponse.json({})),
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () =>
				HttpResponse.json({ authenticatedComponents: [] })
			),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			)
		);
		const { component } = await renderDialog(vmcp);
		await component.openEditConfiguration(vmcp, existing);
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
		await userEvent.keyboard('{Escape}');
		await expect.element(page.getByCSS('input[name="API token"]')).not.toBeInTheDocument();
		component.openReauthenticate(
			vmcp,
			vmcp.components!.map((component) => component.id!),
			existing
		);
		await page.getByRole('button', { name: 'Reauthenticate All' }).click();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await page.getByRole('button', { name: 'Close dialog' }).click();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
	});

	it('hides reauthentication when no component is authenticated', async () => {
		const vmcp = createVMCP({ id: 'vmcp1reauthnone', displayName: 'Remote vMCP' }, [
			createMCPCatalogEntry({ id: 'entry-remote', name: 'Remote', runtime: 'remote' })
		]);
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		const status = vi.fn();
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				status();
				return HttpResponse.json({ authenticatedComponents: [] });
			})
		);

		await renderDialog(vmcp, existing);
		await expect.element(page.getByText('Connection URL', { exact: true })).toBeVisible();
		await vi.waitFor(() => expect(status).toHaveBeenCalledOnce());
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.not.toBeInTheDocument();
	});

	it('clears only the chosen component and prompts to authenticate it', async () => {
		const vmcp = createVMCP({ id: 'vmcp1reauthcomponent', displayName: 'Remote vMCP' }, [
			createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' }),
			createMCPCatalogEntry({ id: 'entry-brave', name: 'Brave Search', runtime: 'remote' })
		]);
		const clearedAll = vi.fn();
		const clearedComponent = vi.fn();
		worker.use(
			http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => {
				clearedAll();
				return HttpResponse.json({});
			}),
			http.delete(`/api/vmcps/${vmcp.id}/components/:componentID/oauth`, ({ params }) => {
				clearedComponent(params.componentID);
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			)
		);

		await preparePageData();
		const { component } = await render(ConnectVMcp);
		void component.reauthenticateComponent(vmcp, undefined, 'component-entry-brave');

		await vi.waitFor(() =>
			expect(clearedComponent).toHaveBeenCalledExactlyOnceWith('component-entry-brave')
		);
		expect(clearedAll).not.toHaveBeenCalled();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();
	});

	describe('leaving the component OAuth prompt', () => {
		async function openComponentPrompt() {
			const vmcp = createVMCP({ id: 'vmcp1reauthdismiss', displayName: 'Remote vMCP' }, [
				createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' })
			]);
			worker.use(
				http.delete(`/api/vmcps/${vmcp.id}/components/:componentID/oauth`, () =>
					HttpResponse.json({})
				),
				http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
					HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
				)
			);
			const onConnected = vi.fn();
			const onDismissed = vi.fn();
			await preparePageData();
			const { component } = await render(ConnectVMcp);
			void component.reauthenticateComponent(vmcp, undefined, 'component-entry-outlook', {
				onConnected,
				onDismissed
			});
			await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
			return { onConnected, onDismissed };
		}

		// Outlasts the dialog's 200ms close animation, after which the native close event fires.
		const closeAnimationSettled = () => new Promise((resolve) => setTimeout(resolve, 400));

		it('finishes the flow when Escape closes the prompt', async () => {
			const { onConnected, onDismissed } = await openComponentPrompt();

			await userEvent.keyboard('{Escape}');

			await vi.waitFor(() => expect(onConnected).toHaveBeenCalledOnce());
			await closeAnimationSettled();
			expect(onConnected).toHaveBeenCalledOnce();
			expect(onDismissed).not.toHaveBeenCalled();
		});

		it('finishes the flow once when the close button is used', async () => {
			const { onConnected } = await openComponentPrompt();

			await page.getByRole('button', { name: 'Close dialog' }).click();

			await vi.waitFor(() => expect(onConnected).toHaveBeenCalledOnce());
			await closeAnimationSettled();
			expect(onConnected).toHaveBeenCalledOnce();
		});
	});
});

it('uses HTTP when any vMCP component requires a localhost callback', async () => {
	const vmcp = configurableVMcp();
	vmcp.components![0].catalogEntry.manifest.remoteConfig = {
		fixedURL: 'https://mcp.example.com',
		localhostCallbackEnabled: true
	};
	await renderDialog(vmcp);
	await expect
		.element(page.getByRole('link', { name: 'Install the Obot CLI' }))
		.not.toBeInTheDocument();
	await expect.element(page.getByText('Connection URL', { exact: true })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'Preconfigure server' })).toBeVisible();
});

it('uses HTTP installation with multiple provider callback paths', async () => {
	const vmcp = configurableVMcp();
	const first = vmcp.components![0];
	first.catalogEntry.manifest.remoteConfig = {
		localhostCallbackEnabled: true,
		localhostCallbackPath: '/custom/callback'
	};
	const second = structuredClone(first);
	second.id = 'second';
	second.name = 'second';
	second.catalogEntry.manifest.remoteConfig!.localhostCallbackPath = '';
	vmcp.components!.push(second);
	await renderDialog(vmcp);
	const link = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(link).toBeVisible();
	const config = JSON.parse(
		atob(new URL(link.element().getAttribute('href')!).searchParams.get('config')!)
	);
	expect(config.type).toBe('http');
	expect(config.url).toContain('/mcp-connect/');
	expect(config).not.toHaveProperty('args');
});

it('uses HTTP installation even with retained callback paths', async () => {
	const vmcp = configurableVMcp();
	vmcp.localhostCallbackPaths = ['/retained/callback'];
	await renderDialog(vmcp);
	const link = page.getByRole('link', { name: /Add to Cursor$/ });
	await expect.element(link).toBeVisible();
	const config = JSON.parse(
		atob(new URL(link.element().getAttribute('href')!).searchParams.get('config')!)
	);
	expect(config).not.toHaveProperty('command');
	expect(config.type).toBe('http');
	expect(config.url).toContain('/mcp-connect/');
	expect(config).not.toHaveProperty('args');
});

it('shows CLI login for vMCP authentication using retained callback paths', async () => {
	const attemptURL = `${window.location.origin}/oauth/mcp/login/ui-attempt`;
	let checks = 0;
	worker.use(
		http.get('*/api/*/oauth-url', () =>
			HttpResponse.json({ oauthURL: ++checks === 1 ? attemptURL : '' })
		)
	);

	const vmcp = configurableVMcp();
	vmcp.localhostCallbackPaths = ['/retained/callback'];
	const result = await renderDialog(vmcp);
	await result.component.authenticate();
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command').last())
		.toHaveTextContent(`obot mcp login "${attemptURL}"`);
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.not.toBeInTheDocument();
});

it('opens component login commands after Authenticate during vMCP preconfiguration', async () => {
	const vmcp = configurableVMcp();
	vmcp.localhostCallbackPaths = ['/oauth/callback'];
	const checked = vi.fn();
	const attemptURL = `${window.location.origin}/oauth/mcp/login/component-state`;
	worker.use(
		http.get('/api/vmcps/vmcp1configurable/oauth-url', () =>
			HttpResponse.json({
				oauthURL: 'https://obot.example/auth/mcp/composite/vmcpi-exact?vmcp_id=vmcp1configurable'
			})
		),
		http.get('/api/vmcps/vmcp1configurable', () => HttpResponse.json(vmcp)),
		http.get('/api/oauth/vmcp/vmcpi-exact', () =>
			HttpResponse.json([{ mcpServerID: 'component-exact', authURL: attemptURL }])
		),
		http.get('/api/oauth/vmcp/vmcpi-exact/components/component-exact', () => {
			checked();
			return HttpResponse.json({ authURL: '' });
		})
	);
	const result = await renderDialog(vmcp);
	await result.component.authenticate();
	await page.getByRole('button', { name: 'Authenticate', exact: true }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login "${attemptURL}"`);
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.not.toBeInTheDocument();
	worker.use(http.get('*/oauth/mcp/login/:state', () => new HttpResponse(null, { status: 400 })));
	document.dispatchEvent(new Event('visibilitychange'));
	await vi.waitFor(() => expect(checked).toHaveBeenCalledOnce());
	await expect.element(page.getByLabelText('Authentication command')).not.toBeInTheDocument();
});

beforeEach(() => {
	worker.use(http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})));
});
