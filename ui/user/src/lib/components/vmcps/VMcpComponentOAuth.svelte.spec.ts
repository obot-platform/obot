import { Group, type VMCP, type VMCPPreconfiguredOAuth } from '$lib/services';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpComponentOAuth from './VMcpComponentOAuth.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// Preconfigured OAuth is the vMCP's, per remote component: one account that everyone who uses the
// vMCP, users and agents, connects upstream as.
const OAUTH_PATH = '/api/vmcps/vmcp-1/components/github/preconfigured-oauth';

function createSharedVMcp(overrides: Partial<VMCP> = {}): VMCP {
	const github = createMCPCatalogEntry({ id: 'github', name: 'GitHub', runtime: 'remote' });
	return createVMCP(
		{
			id: 'vmcp-1',
			displayName: 'Engineering vMCP',
			components: [createVMCPComponent(github, { id: 'github' })],
			...overrides
		},
		[github]
	);
}

function stateOf(oauth: Partial<VMCPPreconfiguredOAuth> = {}): VMCPPreconfiguredOAuth {
	return { vmcpID: 'vmcp-1', componentID: 'github', state: 'needs_authorization', ...oauth };
}

const authorized = (oauth: Partial<VMCPPreconfiguredOAuth> = {}) =>
	stateOf({ state: 'authorized', authorizedBy: 'u1', ...oauth });

function mockState(oauth: VMCPPreconfiguredOAuth = stateOf()) {
	worker.use(http.get(OAUTH_PATH, () => HttpResponse.json(oauth)));
}

async function renderOAuth(
	options: {
		groups?: string[];
		vmcp?: VMCP;
		userName?: (id: string) => string | undefined;
	} = {}
) {
	await preparePageData({ profile: createMockProfile(options.groups ?? [Group.ADMIN]) });
	return render(VMcpComponentOAuth, {
		vmcp: options.vmcp ?? createSharedVMcp(),
		componentID: 'github',
		...(options.userName ? { userName: options.userName } : {})
	});
}

const authorizeButton = () => page.getByRole('button', { name: 'Authorize', exact: true });
const authorizeAgainButton = () =>
	page.getByRole('button', { name: 'Authorize again', exact: true });
const revokeButton = () => page.getByRole('button', { name: 'Revoke', exact: true });

describe('VMcpComponentOAuth', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	const warning = () =>
		page.getByText(
			/Everyone who uses this vMCP, users and agents alike, acts in GitHub as the account that authorizes it/
		);

	// A grant exists, working or not: everyone using the vMCP acts upstream as its account.
	it.each(['authorized', 'invalid'] as const)(
		'warns that everyone acts upstream as the account when it is %s',
		async (state) => {
			mockState(authorized({ state }));
			await renderOAuth();

			await expect
				.element(page.getByRole('heading', { name: 'Preconfigured OAuth' }))
				.toBeVisible();
			await expect.element(warning()).toHaveAttribute('role', 'note');
			await expect
				.element(page.getByText(/users aren't asked to sign in to GitHub, and their own sign-ins/))
				.toBeVisible();
		}
	);

	// Nothing is preconfigured yet, so no one acts as anyone else's account.
	it.each(['needs_authorization', 'not_required'] as const)(
		'shows no warning when it is %s',
		async (state) => {
			mockState(stateOf({ state }));
			await renderOAuth();

			await expect
				.element(page.getByRole('heading', { name: 'Preconfigured OAuth' }))
				.toBeVisible();
			await expect.element(page.getByRole('status')).toBeVisible();
			await expect.element(warning()).not.toBeInTheDocument();
		}
	);

	describe('states', () => {
		it('shows a component known to need OAuth as not authorized, to authorize', async () => {
			mockState(stateOf({ state: 'needs_authorization' }));
			await renderOAuth();

			await expect.element(page.getByText('Not authorized', { exact: true })).toBeVisible();
			await expect
				.element(page.getByText(/users sign in to it with their own accounts/))
				.toBeVisible();
			await expect.element(authorizeButton()).toBeEnabled();
			await expect.element(revokeButton()).not.toBeInTheDocument();
		});

		it('shows a component not known to need OAuth as not required, and still offers to authorize it', async () => {
			// Obot's probe may not have found that it asks for OAuth yet.
			mockState(stateOf({ state: 'not_required' }));
			await renderOAuth();

			await expect.element(page.getByText('Not required', { exact: true })).toBeVisible();
			await expect
				.element(page.getByText(/Authorize it anyway if it asks only once tools are listed/))
				.toBeVisible();
			await expect.element(authorizeButton()).toBeEnabled();
			await expect.element(revokeButton()).not.toBeInTheDocument();
		});

		it('shows who authorized it and when, for what, and with which scopes', async () => {
			mockState(
				authorized({
					authorizedAt: '2026-02-03T12:00:00Z',
					target: 'https://api.github.example/mcp',
					scopes: ['repo', 'read:org']
				})
			);
			await renderOAuth({ userName: (id) => (id === 'u1' ? 'Ada Admin' : undefined) });

			await expect.element(page.getByText(/^Authorized by Ada Admin on /)).toBeVisible();
			await expect.element(page.getByText('for https://api.github.example/mcp')).toBeVisible();
			await expect.element(page.getByText('Scopes: repo read:org')).toBeVisible();
			// An authorized grant can be revoked, or authorized again to switch accounts.
			await expect.element(revokeButton()).toBeVisible();
			await expect.element(authorizeAgainButton()).toBeEnabled();
			await expect.element(page.getByText(/^Authorize again to switch accounts/)).toBeVisible();
			await expect.element(authorizeButton()).not.toBeInTheDocument();
		});

		it('shows only the hostname when the URL comes from configuration, and never a link', async () => {
			mockState(authorized({ targetHostname: 'tenant.example.com' }));
			await renderOAuth();

			await expect.element(page.getByText('for host tenant.example.com')).toBeVisible();
			await expect
				.element(page.getByRole('link', { name: /tenant\.example\.com/ }))
				.not.toBeInTheDocument();
		});

		it('shows an invalid grant with its reason, to authorize again or revoke', async () => {
			mockState(
				stateOf({
					state: 'invalid',
					authorizedBy: 'u1',
					invalidReason: 'The provider refused to refresh the token'
				})
			);
			await renderOAuth();

			await expect
				.element(page.getByText('Needs authorization: the authorization no longer works'))
				.toBeVisible();
			await expect
				.element(page.getByText('The provider refused to refresh the token'))
				.toBeVisible();
			await expect
				.element(page.getByText(/can't use this component until it's authorized again/))
				.toBeVisible();
			await expect.element(authorizeAgainButton()).toBeEnabled();
			await expect.element(revokeButton()).toBeVisible();
		});
	});

	describe('who can change it', () => {
		it.each([
			{ name: 'an auditor', groups: [Group.USER, Group.AUDITOR] },
			{ name: 'a read-only admin', groups: [Group.USER, Group.ADMIN_READONLY] }
		])('shows $name the state without Authorize or Revoke', async ({ groups }) => {
			mockState(authorized({ authorizedAt: '2026-02-03T12:00:00Z' }));
			await renderOAuth({ groups });

			// Without a name from the directory, who authorized it is shown by ID.
			await expect.element(page.getByText(/^Authorized by u1 on /)).toBeVisible();
			await expect.element(page.getByRole('note')).toBeVisible();
			await expect.element(revokeButton()).not.toBeInTheDocument();
			await expect.element(authorizeButton()).not.toBeInTheDocument();
		});

		it.each([
			{ name: 'an auditor', groups: [Group.USER, Group.AUDITOR] },
			{ name: 'a read-only admin', groups: [Group.USER, Group.ADMIN_READONLY] }
		])('offers $name nothing to authorize on a component that needs it', async ({ groups }) => {
			mockState(stateOf({ state: 'needs_authorization' }));
			await renderOAuth({ groups });

			await expect.element(page.getByText('Not authorized', { exact: true })).toBeVisible();
			await expect.element(authorizeButton()).not.toBeInTheDocument();
		});

		it('disables authorizing on a vMCP that is being deleted', async () => {
			mockState();
			await renderOAuth({ vmcp: createSharedVMcp({ deleted: '2026-02-03T00:00:00Z' }) });

			await expect.element(page.getByText(/is being deleted/)).toBeVisible();
			await expect.element(authorizeButton()).toBeDisabled();
		});
	});

	describe('loading', () => {
		it('shows why the state could not be read, and reads it again on request', async () => {
			let reads = 0;
			worker.use(
				http.get(OAUTH_PATH, () => {
					reads++;
					return reads === 1
						? HttpResponse.json({ error: 'component "github" not found' }, { status: 404 })
						: HttpResponse.json(stateOf());
				})
			);
			await renderOAuth();

			await expect
				.element(page.getByRole('alert'))
				.toHaveTextContent('Couldn\'t load the preconfigured OAuth. component "github" not found');
			await page.getByRole('button', { name: 'Try again' }).click();
			await expect.element(page.getByText('Not authorized', { exact: true })).toBeVisible();
		});
	});

	describe('authorizing', () => {
		it('POSTs the preconfigured OAuth route, opens a popup, and shows the grant once polling sees it', async () => {
			let reads = 0;
			const started = vi.fn();
			worker.use(
				http.get(OAUTH_PATH, () => {
					reads++;
					// The first read loads the section, and the next one is the check that comes right after
					// the popup opens.
					return HttpResponse.json(
						reads < 3
							? stateOf()
							: authorized({
									authorizedAt: '2026-02-03T12:00:00Z',
									target: 'https://api.github.example/mcp'
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
			await renderOAuth({ userName: (id) => (id === 'u1' ? 'Ada Admin' : undefined) });

			await authorizeButton().click();

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
			await expect.element(page.getByText(/^Authorized by Ada Admin on /)).toBeVisible();
			await expect.element(page.getByText(/Complete the authorization/)).not.toBeInTheDocument();
			await expect.element(revokeButton()).toBeVisible();
		});

		it('replaces an authorized grant once polling sees the new authorization, though it stays authorized', async () => {
			let reads = 0;
			const started = vi.fn();
			worker.use(
				http.get(OAUTH_PATH, () => {
					reads++;
					// Until the new authorization completes, everyone keeps using the old account.
					return HttpResponse.json(
						reads < 3
							? authorized({ authorizedBy: 'u1', authorizedAt: '2026-02-03T12:00:00Z' })
							: authorized({ authorizedBy: 'u2', authorizedAt: '2026-03-04T12:00:00Z' })
					);
				}),
				http.post(OAUTH_PATH, () => {
					started();
					return HttpResponse.json({ authURL: 'https://provider.example/authorize?state=again' });
				})
			);
			const open = vi.spyOn(window, 'open').mockReturnValue({ opener: {} } as unknown as Window);
			const names: Record<string, string> = { u1: 'Ada Admin', u2: 'Bo Admin' };
			await renderOAuth({ userName: (id) => names[id] });
			await expect.element(page.getByText(/^Authorized by Ada Admin on /)).toBeVisible();

			await authorizeAgainButton().click();

			await vi.waitFor(() => expect(started).toHaveBeenCalledOnce());
			await vi.waitFor(() =>
				expect(open).toHaveBeenCalledWith(
					'https://provider.example/authorize?state=again',
					'_blank',
					expect.stringContaining('popup')
				)
			);
			await expect.element(page.getByText(/^Authorized by Bo Admin on /)).toBeVisible();
			await expect.element(page.getByText(/Complete the authorization/)).not.toBeInTheDocument();
			await expect.element(authorizeAgainButton()).toBeEnabled();
		});

		it('offers the window again when the browser blocked the popup', async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () => HttpResponse.json({ authURL: 'https://provider.example/a' }))
			);
			const open = vi.spyOn(window, 'open').mockReturnValue(null);
			await renderOAuth();

			await authorizeButton().click();
			await expect.element(page.getByText(/blocked the authorization window/)).toBeVisible();
			await page.getByRole('button', { name: 'Open the authorization window' }).click();
			expect(open).toHaveBeenCalledTimes(2);
		});

		it('never opens a window for an authorization URL that is not a web page', async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () => HttpResponse.json({ authURL: 'javascript:alert(1)' }))
			);
			const open = vi.spyOn(window, 'open').mockReturnValue(null);
			await renderOAuth();

			await authorizeButton().click();
			await expect.element(page.getByText(/blocked the authorization window/)).toBeVisible();
			expect(open).not.toHaveBeenCalled();
		});

		it('shows the CLI command instead of a popup for a localhost callback', async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({
						authURL: `${window.location.origin}/oauth/mcp/login/attempt1`,
						localhostCallback: true
					})
				)
			);
			const open = vi.spyOn(window, 'open');
			await renderOAuth();

			await authorizeButton().click();
			await expect
				.element(page.getByText(/obot mcp login "http.*\/oauth\/mcp\/login\/attempt1"/))
				.toBeVisible();
			await expect.element(page.getByText(/You have 10 minutes/)).toBeVisible();
			expect(open).not.toHaveBeenCalled();
		});

		it('finishes a localhost login when polling sees the grant, without a popup', async () => {
			let reads = 0;
			worker.use(
				http.get(OAUTH_PATH, () => {
					reads++;
					return HttpResponse.json(
						reads < 3
							? stateOf()
							: authorized({
									authorizedAt: '2026-02-03T12:00:00Z',
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
			await renderOAuth();

			await authorizeButton().click();
			await expect.element(page.getByText(/^Authorized by u1 on /)).toBeVisible();
			await expect.element(page.getByText(/You have 10 minutes/)).not.toBeInTheDocument();
			expect(open).not.toHaveBeenCalled();
		});

		// The login attempt is used up once the CLI completes it, before the grant may be read back. A
		// used command makes the state be read, and is not called expired.
		it('reads the grant when the login command has been used, instead of calling it expired', async () => {
			let reads = 0;
			const authorize = vi.fn();
			worker.use(
				http.get(OAUTH_PATH, () => {
					reads++;
					return HttpResponse.json(
						reads < 2
							? stateOf()
							: authorized({
									authorizedAt: '2026-02-03T12:00:00Z',
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
			await renderOAuth();

			await authorizeButton().click();
			await expect.element(page.getByText(/^Authorized by u1 on /)).toBeVisible();
			await expect.element(page.getByText(/This command has expired/)).not.toBeInTheDocument();
			expect(authorize).toHaveBeenCalledTimes(1);
		});

		it("shows the server's reason when the URL needs values the vMCP doesn't fix", async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json(
						{
							error:
								'the upstream URL of component "github" needs __url, which the vMCP doesn\'t fix; set it as fixed on the vMCP or its catalog entry',
							reasons: [{ componentID: 'github', code: 'configuration_required', field: '__url' }]
						},
						{ status: 400 }
					)
				)
			);
			await renderOAuth();

			await authorizeButton().click();
			await expect
				.element(page.getByText(/set it as fixed on the vMCP or its catalog entry$/))
				.toHaveAttribute('role', 'alert');
			// Nothing else is blocked: it can be tried again once the value is fixed.
			await expect.element(authorizeButton()).toBeEnabled();
		});

		it("disables authorizing with the server's reason when it is refused as a conflict", async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({ error: 'vMCP is being deleted' }, { status: 409 })
				)
			);
			await renderOAuth();

			await authorizeButton().click();
			await expect.element(page.getByText('vMCP is being deleted').first()).toBeVisible();
			await expect.element(authorizeButton()).toBeDisabled();
		});

		it('shows a component that is gone when authorizing it', async () => {
			mockState();
			worker.use(
				http.post(OAUTH_PATH, () =>
					HttpResponse.json({ error: 'component "github" not found' }, { status: 404 })
				)
			);
			await renderOAuth();

			await authorizeButton().click();
			await expect
				.element(page.getByRole('alert'))
				.toHaveTextContent('component "github" not found');
		});
	});

	describe('revoking', () => {
		it('DELETEs the preconfigured OAuth route after a confirmation, and shows the state again', async () => {
			let revoked = false;
			const deleted = vi.fn();
			worker.use(
				http.get(OAUTH_PATH, () =>
					HttpResponse.json(revoked ? stateOf({ state: 'needs_authorization' }) : authorized())
				),
				http.delete(OAUTH_PATH, () => {
					deleted();
					revoked = true;
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderOAuth();

			await revokeButton().click();
			await expect
				.element(page.getByText(/will be asked to sign in to GitHub with their own accounts/))
				.toBeVisible();
			expect(deleted).not.toHaveBeenCalled();
			await revokeButton().last().click();
			await vi.waitFor(() => expect(deleted).toHaveBeenCalledOnce());
			await expect.element(page.getByText('Not authorized', { exact: true })).toBeVisible();
			await expect.element(authorizeButton()).toBeVisible();
		});

		it('keeps the grant when the confirmation is cancelled', async () => {
			mockState(authorized());
			const deleted = vi.fn();
			worker.use(
				http.delete(OAUTH_PATH, () => {
					deleted();
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderOAuth();

			await revokeButton().click();
			await page.getByRole('button', { name: 'Cancel', exact: true }).click();
			await expect.element(page.getByText('Authorized by u1', { exact: true })).toBeVisible();
			expect(deleted).not.toHaveBeenCalled();
		});

		it("shows the server's reason when revoking is refused", async () => {
			mockState(authorized());
			worker.use(
				http.delete(OAUTH_PATH, () =>
					HttpResponse.json({ error: 'vMCP is being deleted' }, { status: 409 })
				)
			);
			await renderOAuth();

			await revokeButton().click();
			await revokeButton().last().click();
			await expect.element(page.getByText('vMCP is being deleted').first()).toBeVisible();
			await expect.element(page.getByText('Authorized by u1', { exact: true })).toBeVisible();
		});
	});
});
