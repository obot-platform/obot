import { page as appPage } from '$app/state';
import type {
	VMCP,
	VMCPAgentOAuthStatus,
	VMCPAgentSetup,
	VMCPAgentSetupComponent
} from '$lib/services';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import VMcpAgentSetup from './VMcpAgentSetup.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

// Agent setup is the vMCP's: one grant per component, shared by every agent granted it.
const SETUP_PATH = '/api/vmcps/vmcp-1/agent-setup';
const OAUTH_PATH = '/api/vmcps/vmcp-1/agent-oauth/github';

function createSetupVMcp(): VMCP {
	const github = createMCPCatalogEntry({
		id: 'github',
		name: 'GitHub',
		runtime: 'remote',
		env: [
			{
				key: 'API_TOKEN',
				name: 'API token',
				description: 'Token for the API',
				required: true,
				sensitive: true,
				value: ''
			}
		]
	});
	const jira = createMCPCatalogEntry({ id: 'jira', name: 'Jira', runtime: 'remote' });
	const files = createMCPCatalogEntry({ id: 'files', name: 'Files' });
	return createVMCP(
		{
			id: 'vmcp-1',
			displayName: 'Engineering vMCP',
			components: [
				createVMCPComponent(github, {
					id: 'github',
					configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
				}),
				createVMCPComponent(jira, { id: 'jira' }),
				createVMCPComponent(files, { id: 'files' })
			],
			profiles: [
				{
					id: 'vp1bots',
					name: 'Bots',
					subjects: [{ type: 'agent', id: '5' }],
					vmcpPermissions: { allowedComponents: { github: { allowedTools: ['*'] } } }
				}
			]
		},
		[github, jira, files]
	);
}

// The setup of GitHub, ready unless the reasons say otherwise.
function github(
	oauth: VMCPAgentOAuthStatus | undefined,
	reasons: VMCPAgentSetupComponent['reasons'] = []
): VMCPAgentSetupComponent {
	return { componentID: 'github', ready: reasons.length === 0, reasons, oauth };
}

function setupOf(...components: VMCPAgentSetupComponent[]): VMCPAgentSetup {
	return { vmcpID: 'vmcp-1', components };
}

const needsOAuth = () =>
	github({ required: true, state: 'missing' }, [{ componentID: 'github', code: 'oauth_required' }]);

function mockSetup(setup: VMCPAgentSetup = setupOf(needsOAuth())) {
	worker.use(http.get(SETUP_PATH, () => HttpResponse.json(setup)));
}

function renderSetup(userName?: (id: string) => string | undefined) {
	return render(VMcpAgentSetup, { vmcp: createSetupVMcp(), ...(userName ? { userName } : {}) });
}

describe('VMcpAgentSetup', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('lists only the components the profiles naming agents grant', async () => {
		// The profile grants GitHub only, so the vMCP's agent setup lists GitHub only.
		mockSetup();
		renderSetup();

		await expect.element(page.getByRole('heading', { name: 'Agent setup' })).toBeVisible();
		await expect.element(page.getByRole('region', { name: 'GitHub' })).toBeVisible();
		await expect.element(page.getByRole('region', { name: 'Jira' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('region', { name: 'Files' })).not.toBeInTheDocument();
	});

	it('says so when the profiles naming agents grant no components', async () => {
		mockSetup(setupOf());
		renderSetup();

		await expect
			.element(page.getByText('The profiles that name agents grant no components yet.'))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Authorize', exact: true }))
			.not.toBeInTheDocument();
	});

	describe('OAuth requirement', () => {
		it('shows a component that needs OAuth and has no grant as needing authorization', async () => {
			// Obot's probe found that the server asks for OAuth, and the vMCP holds no grant yet.
			mockSetup();
			renderSetup();

			await expect.element(page.getByText(/Not ready/)).toBeVisible();
			await expect.element(page.getByText('Needs authorization', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Authorize GitHub with OAuth' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Authorize', exact: true }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
		});

		it('shows a remote component not known to need OAuth as not required, and still offers to authorize it', async () => {
			// Nothing shows that it asks for OAuth, as when its probe failed: agents are not kept from it.
			mockSetup(setupOf(github({ required: false, state: 'missing' })));
			renderSetup();

			await expect.element(page.getByText(/Ready: agents can use every component/)).toBeVisible();
			const group = page.getByRole('region', { name: 'GitHub' });
			await expect.element(group.getByText('Not required', { exact: true })).toBeVisible();
			await expect
				.element(group.getByRole('button', { name: 'Authorize', exact: true }))
				.toBeVisible();
		});

		it('shows OAuth as not required, with nothing to authorize, for a component that is not remote', async () => {
			mockSetup(setupOf({ componentID: 'files', ready: true, reasons: [] }));
			renderSetup();

			const group = page.getByRole('region', { name: 'Files' });
			await expect.element(group.getByText('Not required', { exact: true })).toBeVisible();
			await expect.element(group.getByRole('button')).not.toBeInTheDocument();
		});
	});

	it('offers no configuration: agents get only the values the vMCP or its catalog entry fix', async () => {
		mockSetup();
		renderSetup();

		await expect
			.element(
				page.getByRole('region', { name: 'GitHub' }).getByRole('heading', { name: 'GitHub' })
			)
			.toBeVisible();
		await expect.element(page.getByRole('textbox')).not.toBeInTheDocument();
		await expect.element(page.getByLabelText(/API token/)).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Save configuration' }))
			.not.toBeInTheDocument();
	});

	it('shows a component that needs a value agents cannot get as not ready, with where to fix it', async () => {
		mockSetup(
			setupOf(
				github({ required: false, state: 'missing' }, [
					{ componentID: 'github', code: 'configuration_required', field: 'API_TOKEN' },
					{ componentID: 'github', code: 'configuration_required', field: '__url' }
				])
			)
		);
		renderSetup();

		await expect.element(page.getByText(/Not ready/)).toBeVisible();
		await expect
			.element(
				page.getByRole('button', {
					name: "Set API token of GitHub as fixed on the vMCP or its catalog entry, or stop granting GitHub to agents: agents can't set it"
				})
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Set Server URL of GitHub as fixed/ }))
			.toBeVisible();
		const group = page.getByRole('region', { name: 'GitHub' });
		await expect
			.element(
				group.getByText(
					/Needs a value fixed on the vMCP or its catalog entry: API token, Server URL\./
				)
			)
			.toBeVisible();
		await expect.element(page.getByText(/Ready: agents can use/)).not.toBeInTheDocument();
	});

	it('shows a component whose configuration is all fixed as ready', async () => {
		mockSetup(setupOf(github({ required: false, state: 'missing' })));
		renderSetup();

		await expect.element(page.getByText(/Ready: agents get every value they need/)).toBeVisible();
		await expect.element(page.getByText(/agents can't set it/)).not.toBeInTheDocument();
	});

	it('links each missing item to the component or button it is about', async () => {
		mockSetup(
			setupOf(
				github({ required: true, state: 'missing' }, [
					{ componentID: 'github', code: 'configuration_required', field: 'API_TOKEN' },
					{ componentID: 'github', code: 'oauth_required' }
				])
			)
		);
		renderSetup();

		await expect.element(page.getByText(/Not ready/)).toBeVisible();
		await page.getByRole('button', { name: /Set API token of GitHub as fixed/ }).click();
		await expect.element(page.getByRole('region', { name: 'GitHub' })).toHaveFocus();
		await page.getByRole('button', { name: 'Authorize GitHub with OAuth' }).click();
		await expect
			.element(page.getByRole('button', { name: 'Authorize', exact: true }))
			.toHaveFocus();
	});

	it('lands on the button a link from the agent page names', async () => {
		mockSetup();
		(appPage.url as URL).hash = `#${encodeURIComponent('agent-oauth-github')}`;
		try {
			renderSetup();
			await expect
				.element(page.getByRole('button', { name: 'Authorize', exact: true }))
				.toHaveFocus();
		} finally {
			(appPage.url as URL).hash = '';
		}
	});

	it('disables authorizing with the server’s reason when it is refused as a conflict', async () => {
		mockSetup();
		worker.use(
			http.post(OAUTH_PATH, () =>
				HttpResponse.json({ error: 'vMCP is being deleted' }, { status: 409 })
			)
		);
		renderSetup();

		await page.getByRole('button', { name: 'Authorize', exact: true }).click();
		await expect.element(page.getByText('vMCP is being deleted').first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Authorize', exact: true }))
			.toBeDisabled();
	});

	describe('OAuth', () => {
		const authorized = (oauth: Partial<VMCPAgentOAuthStatus> = {}) =>
			setupOf(github({ required: true, state: 'authorized', authorizedBy: 'u1', ...oauth }));

		it('authorizes on the vMCP in a popup and shows the grant once polling sees it change', async () => {
			let polls = 0;
			const started = vi.fn();
			worker.use(
				http.get(SETUP_PATH, () => {
					polls++;
					// The first read is the one that loads the section, and the next one the check
					// that comes right after the popup opens.
					return HttpResponse.json(
						polls < 3
							? setupOf(needsOAuth())
							: authorized({
									authorizedAt: '2026-02-03T00:00:00Z',
									target: 'https://api.github.example/mcp',
									scopes: ['repo', 'read:org']
								})
					);
				}),
				http.post(OAUTH_PATH, () => {
					started();
					return HttpResponse.json({ authURL: 'https://provider.example/authorize?state=abc' });
				})
			);
			const popup = { opener: {} } as unknown as Window;
			const open = vi.spyOn(window, 'open').mockReturnValue(popup);
			renderSetup((id) => (id === 'u1' ? 'Ada Admin' : undefined));

			await expect.element(page.getByText('Needs authorization', { exact: true })).toBeVisible();
			await page.getByRole('button', { name: 'Authorize', exact: true }).click();

			await vi.waitFor(() => expect(started).toHaveBeenCalledOnce());
			await vi.waitFor(() =>
				expect(open).toHaveBeenCalledWith(
					'https://provider.example/authorize?state=abc',
					'_blank',
					expect.stringContaining('popup')
				)
			);
			// The provider's page must not be able to reach back into this one.
			expect(popup.opener).toBeNull();
			await expect.element(page.getByText(/Authorized by Ada Admin on/)).toBeVisible();
			await expect.element(page.getByText('for https://api.github.example/mcp')).toBeVisible();
			await expect.element(page.getByText('Scopes: repo read:org')).toBeVisible();
			await expect.element(page.getByText(/Complete the authorization/)).not.toBeInTheDocument();
			// An authorized grant is revoked, not authorized again.
			await expect.element(page.getByRole('button', { name: 'Revoke' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Authorize', exact: true }))
				.not.toBeInTheDocument();
		});

		it('offers the window again when the browser blocked the popup', async () => {
			mockSetup();
			worker.use(
				http.post(OAUTH_PATH, () => HttpResponse.json({ authURL: 'https://provider.example/a' }))
			);
			const open = vi.spyOn(window, 'open').mockReturnValue(null);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/blocked the authorization window/)).toBeVisible();
			await page.getByRole('button', { name: 'Open the authorization window' }).click();
			expect(open).toHaveBeenCalledTimes(2);
		});

		it('never opens a window for an authorization URL that is not a web page', async () => {
			mockSetup();
			worker.use(
				http.post(OAUTH_PATH, () => HttpResponse.json({ authURL: 'javascript:alert(1)' }))
			);
			const open = vi.spyOn(window, 'open').mockReturnValue(null);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/blocked the authorization window/)).toBeVisible();
			expect(open).not.toHaveBeenCalled();
		});

		it('shows the CLI command instead of a popup for a localhost callback', async () => {
			mockSetup();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({
						authURL: `${window.location.origin}/oauth/mcp/login/attempt1`,
						localhostCallback: true
					})
				)
			);
			const open = vi.spyOn(window, 'open');
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect
				.element(page.getByText(/obot mcp login "http.*\/oauth\/mcp\/login\/attempt1"/))
				.toBeVisible();
			await expect.element(page.getByText(/You have 10 minutes/)).toBeVisible();
			expect(open).not.toHaveBeenCalled();
		});

		it('finishes a localhost login when polling sees the grant, without a popup', async () => {
			let polls = 0;
			worker.use(
				http.get(SETUP_PATH, () => {
					polls++;
					return HttpResponse.json(
						polls < 3
							? setupOf(needsOAuth())
							: authorized({
									authorizedAt: '2026-02-03T00:00:00Z',
									targetHostname: 'tenant.example.com'
								})
					);
				}),
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({
						authURL: `${window.location.origin}/oauth/mcp/login/attempt1`,
						localhostCallback: true
					})
				)
			);
			const open = vi.spyOn(window, 'open');
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/Authorized by u1 on/)).toBeVisible();
			await expect.element(page.getByText(/You have 10 minutes/)).not.toBeInTheDocument();
			expect(open).not.toHaveBeenCalled();
		});

		// The login attempt is used up once the CLI completes it, before the grant may be read back. A used
		// command makes the setup be read, and is not called expired.
		it('reads the grant when the login command has been used, instead of calling it expired', async () => {
			let polls = 0;
			const authorize = vi.fn();
			worker.use(
				http.get(SETUP_PATH, () => {
					polls++;
					return HttpResponse.json(
						polls < 2
							? setupOf(needsOAuth())
							: authorized({
									authorizedAt: '2026-02-03T00:00:00Z',
									targetHostname: 'tenant.example.com'
								})
					);
				}),
				http.post(OAUTH_PATH, () => {
					authorize();
					return HttpResponse.json({
						authURL: `${window.location.origin}/oauth/mcp/login/used1`,
						localhostCallback: true
					});
				}),
				http.get('/oauth/mcp/login/used1', () => new HttpResponse(null, { status: 400 }))
			);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/Authorized by u1 on/)).toBeVisible();
			await expect.element(page.getByText(/This command has expired/)).not.toBeInTheDocument();
			expect(authorize).toHaveBeenCalledTimes(1);
		});

		it('shows only the hostname when the URL comes from configuration, and never a link', async () => {
			mockSetup(authorized({ targetHostname: 'tenant.example.com' }));
			renderSetup();

			await expect.element(page.getByText('for host tenant.example.com')).toBeVisible();
			await expect
				.element(page.getByRole('link', { name: /tenant\.example\.com/ }))
				.not.toBeInTheDocument();
		});

		it('shows an invalid grant with its reason, to authorize again or revoke', async () => {
			mockSetup(
				setupOf(
					github(
						{
							required: true,
							state: 'invalid',
							invalidReason: 'The provider refused to refresh the token'
						},
						[{ componentID: 'github', code: 'oauth_invalid' }]
					)
				)
			);
			renderSetup();

			await expect
				.element(page.getByText('Needs authorization: the authorization no longer works'))
				.toBeVisible();
			await expect
				.element(page.getByText('The provider refused to refresh the token'))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: /Authorize GitHub again/ }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Authorize again' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Revoke' })).toBeVisible();
		});

		it('shows the error as returned when the URL is not fixed, without fields to set', async () => {
			mockSetup();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json(
						{
							error:
								'the upstream URL of component "github" needs __url, which agents can\'t get; set it as fixed on the vMCP or its catalog entry, or stop granting the component to agents',
							reasons: [{ componentID: 'github', code: 'configuration_required', field: '__url' }]
						},
						{ status: 400 }
					)
				)
			);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/set it as fixed on the vMCP/)).toBeVisible();
			await expect.element(page.getByRole('textbox')).not.toBeInTheDocument();
		});

		it('revokes the vMCP’s grant after a confirmation', async () => {
			mockSetup(authorized());
			const revoked = vi.fn();
			worker.use(
				http.delete(OAUTH_PATH, () => {
					revoked();
					return HttpResponse.json(setupOf(needsOAuth()));
				})
			);
			renderSetup();

			await page.getByRole('button', { name: 'Revoke', exact: true }).click();
			await expect
				.element(page.getByText(/is refused the component until it is authorized again/))
				.toBeVisible();
			await page.getByRole('button', { name: 'Revoke', exact: true }).last().click();
			await vi.waitFor(() => expect(revoked).toHaveBeenCalledOnce());
			await expect.element(page.getByText('Needs authorization', { exact: true })).toBeVisible();
		});
	});
});
