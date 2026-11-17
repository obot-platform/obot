import { page as appPage } from '$app/state';
import type { VMCP, VMCPProfileStatus } from '$lib/services';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import VMcpProfileAgentSetup from './VMcpProfileAgentSetup.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

const STATUS_PATH = '/api/vmcps/vmcp-1/profiles/vp1abc/status';
const CONFIGURATION_PATH = '/api/vmcps/vmcp-1/profiles/vp1abc/configuration';
const OAUTH_PATH = '/api/vmcps/vmcp-1/profiles/vp1abc/components/github/oauth';

function createSetupVMcp(overrides: Partial<VMCP> = {}): VMCP {
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
			},
			{
				key: 'REGION',
				name: 'Region',
				description: '',
				required: false,
				sensitive: false,
				value: ''
			},
			{
				key: 'ORG',
				name: 'Organization',
				description: '',
				required: false,
				sensitive: false,
				value: ''
			}
		]
	});
	return createVMCP(
		{
			id: 'vmcp-1',
			displayName: 'Engineering vMCP',
			components: [
				createVMCPComponent(github, {
					id: 'github',
					configuration: [
						{ key: 'API_TOKEN', policy: 'userAllowed' },
						{ key: 'REGION', policy: 'userAllowed' },
						{ key: 'ORG', policy: 'fixed', value: 'acme' }
					]
				})
			],
			...overrides
		},
		[github]
	);
}

function profileStatus(overrides: Partial<VMCPProfileStatus> = {}): VMCPProfileStatus {
	return {
		profileID: 'vp1abc',
		agentCapable: true,
		components: [
			{
				componentID: 'github',
				configuration: { set: [], missingRequired: ['API_TOKEN'], optionalUnset: ['REGION'] },
				oauth: { required: true, state: 'missing' }
			}
		],
		...overrides
	};
}

function mockStatus(status: VMCPProfileStatus = profileStatus()) {
	worker.use(http.get(STATUS_PATH, () => HttpResponse.json(status)));
}

function renderSetup(vmcp: VMCP = createSetupVMcp(), profileID = 'vp1abc') {
	return render(VMcpProfileAgentSetup, { vmcp, profileID });
}

describe('VMcpProfileAgentSetup', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('is disabled until the profile is saved, and asks for nothing', async () => {
		render(VMcpProfileAgentSetup, { vmcp: createSetupVMcp() });
		await expect.element(page.getByText(/Save the profile first/)).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Save configuration' }))
			.not.toBeInTheDocument();
	});

	it('offers nothing to set up, and no note about subjects, for a profile no agent can use', async () => {
		mockStatus(profileStatus({ agentCapable: false, components: [] }));
		renderSetup();
		await expect.element(page.getByRole('heading', { name: 'Agent setup' })).toBeVisible();
		await expect
			.element(page.getByText(/Add an agent or wildcard subject/))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Save configuration' }))
			.not.toBeInTheDocument();
	});

	it('lists only the components the profile grants, without a note about the others', async () => {
		const jira = createMCPCatalogEntry({ id: 'jira', name: 'Jira', runtime: 'remote' });
		const vmcp = createSetupVMcp();
		vmcp.components = [...(vmcp.components ?? []), createVMCPComponent(jira, { id: 'jira' })];
		// The profile grants GitHub only, so its status lists GitHub only.
		mockStatus();
		renderSetup(vmcp);

		await expect.element(page.getByRole('region', { name: 'GitHub' })).toBeVisible();
		await expect.element(page.getByRole('region', { name: 'Jira' })).not.toBeInTheDocument();
		await expect
			.element(page.getByText(/Every component of the vMCP is listed/))
			.not.toBeInTheDocument();
	});

	describe('OAuth requirement', () => {
		const configured = {
			componentID: 'github',
			configuration: { set: ['API_TOKEN'], missingRequired: [], optionalUnset: ['REGION'] }
		};

		it('shows a component that needs OAuth and has no grant as not ready', async () => {
			// Obot's probe found that the server asks for OAuth, and the profile holds no grant yet.
			mockStatus(
				profileStatus({
					components: [{ ...configured, oauth: { required: true, state: 'missing' } }]
				})
			);
			renderSetup();
			await expect.element(page.getByText(/Not ready/)).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Authorize GitHub with OAuth' }))
				.toBeVisible();
			await expect.element(page.getByText(/Ready: agents can use/)).not.toBeInTheDocument();
		});

		it('shows a component not known to need OAuth as ready, and still offers to authorize it', async () => {
			// Nothing shows that it asks for OAuth, as when its probe failed: agents are not kept from it.
			mockStatus(
				profileStatus({
					components: [{ ...configured, oauth: { required: false, state: 'missing' } }]
				})
			);
			renderSetup();
			await expect.element(page.getByText(/Ready: agents can use/)).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Authorize', exact: true }))
				.toBeVisible();
		});
	});

	it('lists every component with required keys marked, optional keys apart, and fixed values read-only', async () => {
		mockStatus();
		renderSetup();

		const group = page.getByRole('region', { name: 'GitHub' });
		await expect.element(group.getByLabelText(/API token/)).toBeVisible();
		await expect.element(group.getByText('Required', { exact: true }).first()).toBeInTheDocument();
		await expect.element(group.getByText('Optional')).toBeVisible();
		await expect.element(group.getByLabelText('Region')).toBeVisible();
		// A value the vMCP fixes applies to everyone: named, never shown, never an input.
		await expect.element(group.getByText('Applies to everyone')).toBeVisible();
		await expect.element(group.getByText('Organization')).toBeVisible();
		await expect.element(group.getByText('Fixed by the vMCP')).toBeVisible();
		await expect.element(group.getByLabelText('Organization')).not.toBeInTheDocument();
		await expect.element(page.getByText('acme')).not.toBeInTheDocument();
	});

	it('never renders a typed value after it is saved, and only sends what changed', async () => {
		mockStatus();
		const saved = vi.fn();
		worker.use(
			http.put(CONFIGURATION_PATH, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(
					profileStatus({
						components: [
							{
								componentID: 'github',
								configuration: {
									set: ['API_TOKEN'],
									missingRequired: [],
									optionalUnset: ['REGION']
								},
								oauth: { required: true, state: 'missing' }
							}
						]
					})
				);
			})
		);
		renderSetup();

		const secret = 'ghp_super-secret-value';
		await page.getByLabelText(/API token/).fill(secret);
		// A blank value would clear the key on the server, so an untouched key is left out.
		await page.getByLabelText('Region').fill('   ');
		await page.getByRole('button', { name: 'Save configuration' }).click();

		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({ components: { github: { API_TOKEN: secret } } })
		);
		// The key now only shows as set, with no input and no value anywhere on the page.
		await expect.element(page.getByRole('button', { name: 'Replace' })).toBeVisible();
		await expect.element(page.getByLabelText(/API token/)).not.toBeInTheDocument();
		await expect.element(page.getByText(secret)).not.toBeInTheDocument();
		expect(document.body.innerText).not.toContain(secret);
		await expect.element(page.getByLabelText('Region')).toHaveValue('');
	});

	it('replaces and clears a set key with Replace and Clear, sending null for a clear', async () => {
		mockStatus(
			profileStatus({
				components: [
					{
						componentID: 'github',
						configuration: { set: ['API_TOKEN', 'REGION'], missingRequired: [], optionalUnset: [] },
						oauth: { required: true, state: 'missing' }
					}
				]
			})
		);
		const saved = vi.fn();
		worker.use(
			http.put(CONFIGURATION_PATH, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(profileStatus());
			})
		);
		renderSetup();

		// A set key shows no input and no value until it is replaced.
		await expect.element(page.getByLabelText(/API token/)).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Replace' }).first().click();
		await page.getByLabelText(/API token/).fill('new-token');
		await page.getByRole('button', { name: 'Clear' }).click();
		await expect.element(page.getByText('Will be cleared')).toBeVisible();
		await page.getByRole('button', { name: 'Save configuration' }).click();

		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({
				components: { github: { API_TOKEN: 'new-token', REGION: null } }
			})
		);
	});

	it('links each missing item to the field or button that fixes it', async () => {
		mockStatus();
		renderSetup();

		await expect.element(page.getByText(/Not ready/)).toBeVisible();
		await page.getByRole('button', { name: 'Set API token for GitHub' }).click();
		await expect.element(page.getByLabelText(/API token/)).toHaveFocus();
		await page.getByRole('button', { name: 'Authorize GitHub with OAuth' }).click();
		await expect
			.element(page.getByRole('button', { name: 'Authorize', exact: true }))
			.toHaveFocus();
	});

	it('lands on the field a link from the agent page names', async () => {
		mockStatus();
		(appPage.url as URL).hash = `#${encodeURIComponent('agent-config-github-API_TOKEN')}`;
		try {
			renderSetup();
			await expect.element(page.getByLabelText(/API token/)).toHaveFocus();
		} finally {
			(appPage.url as URL).hash = '';
		}
	});

	it('shows the ready state and the cleanup that is still pending after a reset', async () => {
		mockStatus(
			profileStatus({
				components: [
					{
						componentID: 'github',
						configuration: { set: ['API_TOKEN'], missingRequired: [], optionalUnset: ['REGION'] },
						oauth: { required: false, state: 'missing' }
					}
				]
			})
		);
		worker.use(
			http.post(`${CONFIGURATION_PATH}/reset`, () =>
				HttpResponse.json(profileStatus({ pendingCollection: true }))
			)
		);
		renderSetup();

		await expect.element(page.getByText(/Ready: agents can use/)).toBeVisible();
		await expect.element(page.getByText(/still being removed/)).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Reset configuration' }).click();
		await page.getByRole('button', { name: 'Reset', exact: true }).click();
		await expect.element(page.getByText(/still being removed/)).toBeVisible();
	});

	it('explains an ambiguous selection and leaves the values read-only', async () => {
		mockStatus();
		const vmcp = createSetupVMcp({
			status: {
				ready: true,
				conditions: [
					{
						type: 'AgentSelectionAmbiguous',
						status: 'True',
						lastTransitionTime: '2026-01-01T00:00:00Z'
					}
				]
			}
		});
		renderSetup(vmcp);

		await expect.element(page.getByText('More than one profile names every agent')).toBeVisible();
		await expect.element(page.getByLabelText(/API token/)).toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Authorize', exact: true }))
			.toBeDisabled();
	});

	it('disables editing with the server’s reason when a save is refused as a conflict', async () => {
		mockStatus();
		worker.use(
			http.put(CONFIGURATION_PATH, () =>
				HttpResponse.json({ error: 'vMCP is being deleted' }, { status: 409 })
			)
		);
		renderSetup();

		await page.getByLabelText(/API token/).fill('x');
		await page.getByRole('button', { name: 'Save configuration' }).click();
		await expect.element(page.getByText('vMCP is being deleted').first()).toBeVisible();
		await expect.element(page.getByLabelText(/API token/)).toBeDisabled();
	});

	describe('OAuth', () => {
		function oauthStatus(oauth: NonNullable<VMCPProfileStatus['components'][number]['oauth']>) {
			return profileStatus({
				components: [
					{
						componentID: 'github',
						configuration: { set: ['API_TOKEN'], missingRequired: [], optionalUnset: [] },
						oauth
					}
				]
			});
		}

		it('opens the provider in a popup and shows the grant once polling sees it change', async () => {
			let polls = 0;
			worker.use(
				http.get(STATUS_PATH, () => {
					polls++;
					// The first read is the one that loads the section, and the next one the check
					// that comes right after the popup opens.
					return HttpResponse.json(
						polls < 3
							? oauthStatus({ required: true, state: 'missing' })
							: oauthStatus({
									required: true,
									state: 'authorized',
									authorizedBy: 'u1',
									authorizedAt: '2026-02-03T00:00:00Z',
									target: 'https://api.github.example/mcp'
								})
					);
				}),
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({ authURL: 'https://provider.example/authorize?state=abc' })
				)
			);
			const popup = { opener: {} } as unknown as Window;
			const open = vi.spyOn(window, 'open').mockReturnValue(popup);
			render(VMcpProfileAgentSetup, {
				vmcp: createSetupVMcp(),
				profileID: 'vp1abc',
				userName: (id: string) => (id === 'u1' ? 'Ada Admin' : undefined)
			});

			await expect.element(page.getByText('Not authorized')).toBeVisible();
			await page.getByRole('button', { name: 'Authorize', exact: true }).click();

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
			await expect.element(page.getByText(/Complete the authorization/)).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Authorize again' })).toBeVisible();
		});

		it('offers the window again when the browser blocked the popup', async () => {
			mockStatus(oauthStatus({ required: true, state: 'missing' }));
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
			mockStatus(oauthStatus({ required: true, state: 'missing' }));
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
			mockStatus(oauthStatus({ required: true, state: 'missing' }));
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
				http.get(STATUS_PATH, () => {
					polls++;
					return HttpResponse.json(
						oauthStatus(
							polls < 3
								? { required: true, state: 'missing' }
								: {
										required: true,
										state: 'authorized',
										authorizedBy: 'u1',
										authorizedAt: '2026-02-03T00:00:00Z',
										targetHostname: 'tenant.example.com'
									}
						)
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
		// command makes the status be read, and is not called expired.
		it('reads the grant when the login command has been used, instead of calling it expired', async () => {
			let polls = 0;
			const authorize = vi.fn();
			worker.use(
				http.get(STATUS_PATH, () => {
					polls++;
					return HttpResponse.json(
						oauthStatus(
							polls < 2
								? { required: true, state: 'missing' }
								: {
										required: true,
										state: 'authorized',
										authorizedBy: 'u1',
										authorizedAt: '2026-02-03T00:00:00Z',
										targetHostname: 'tenant.example.com'
									}
						)
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
			mockStatus(
				oauthStatus({
					required: true,
					state: 'authorized',
					authorizedBy: 'u1',
					targetHostname: 'tenant.example.com'
				})
			);
			renderSetup();

			await expect.element(page.getByText('for host tenant.example.com')).toBeVisible();
			await expect
				.element(page.getByRole('link', { name: /tenant\.example\.com/ }))
				.not.toBeInTheDocument();
		});

		it('shows an invalid grant with its reason', async () => {
			mockStatus(
				oauthStatus({
					required: true,
					state: 'invalid',
					invalidReason: 'The provider refused to refresh the token'
				})
			);
			renderSetup();

			await expect.element(page.getByText('Authorization no longer works')).toBeVisible();
			await expect
				.element(page.getByText('The provider refused to refresh the token'))
				.toBeVisible();
		});

		it('shows the error as returned, placeholders included, and points at the fields the server names', async () => {
			mockStatus(profileStatus());
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json(
						{
							error:
								'the upstream URL of component "github" needs <configured URL>, which profile "vp1abc" does not configure; configure the profile first',
							reasons: [
								{
									componentID: 'github',
									code: 'configuration_required',
									field: 'REGION',
									profileID: 'vp1abc'
								}
							]
						},
						{ status: 400 }
					)
				)
			);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/needs <configured URL>/)).toBeVisible();
			// The server named Region, not the first required key that is missing.
			await expect
				.element(page.getByRole('button', { name: 'Go to API token' }))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Go to Region' }).click();
			await expect.element(page.getByLabelText('Region')).toHaveFocus();
		});

		it('shows the error as returned, placeholders included, and falls back to the first missing required key when it names none', async () => {
			mockStatus(profileStatus());
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json(
						{
							error:
								'the upstream URL of component "github" needs <configured URL>, which profile "vp1abc" does not configure; configure the profile first'
						},
						{ status: 400 }
					)
				)
			);
			renderSetup();

			await page.getByRole('button', { name: 'Authorize', exact: true }).click();
			await expect.element(page.getByText(/needs <configured URL>/)).toBeVisible();
			await page.getByRole('button', { name: 'Go to API token' }).click();
			await expect.element(page.getByLabelText(/API token/)).toHaveFocus();
		});

		it('resets a grant after a confirmation', async () => {
			mockStatus(oauthStatus({ required: true, state: 'authorized', authorizedBy: 'u1' }));
			const reset = vi.fn();
			worker.use(
				http.delete(OAUTH_PATH, () => {
					reset();
					return HttpResponse.json(oauthStatus({ required: true, state: 'missing' }));
				})
			);
			renderSetup();

			await page.getByRole('button', { name: 'Reset', exact: true }).click();
			await expect
				.element(page.getByText(/is refused the component until it is authorized again/))
				.toBeVisible();
			await page.getByRole('button', { name: 'Reset', exact: true }).last().click();
			await vi.waitFor(() => expect(reset).toHaveBeenCalled());
			await expect.element(page.getByText('Not authorized')).toBeVisible();
		});
	});
});
