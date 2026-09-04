import type { MCPCatalogEntry, ToolOverride } from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import {
	createMCPCatalogEntry,
	createVMCP,
	createVMCPComponent,
	type VMCPTestResource
} from '../../tests/helpers/mcp';
import { preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import VMcpsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub',
	manifest: {
		toolPreview: [
			{ id: 'create_issue', name: 'create_issue', description: 'Create an issue' },
			{ id: 'list_issues', name: 'list_issues', description: 'List issues' }
		]
	}
});

const toolOverrides: ToolOverride[] = [
	{ name: 'create_issue', description: 'Create an issue', enabled: true },
	{ name: 'list_issues', description: 'List issues', enabled: false }
];

const remoteToolPreview = [
	{ id: 'search', name: 'search', description: 'Search records' },
	{ id: 'list', name: 'list', description: 'List records' }
];

const remoteEntry = createMCPCatalogEntry({
	id: 'entry-remote',
	name: 'Remote Records',
	runtime: 'remote',
	manifest: { toolPreview: remoteToolPreview }
});

const remoteToolOverrides: ToolOverride[] = [
	{
		name: 'search',
		overrideName: 'find_records',
		overrideDescription: 'Find records',
		enabled: true
	},
	{ name: 'list', description: 'List records', enabled: false }
];

function createVMcp(overrides?: ToolOverride[], id = 'vmcp-1') {
	return createVMCP(
		{
			id,
			displayName: id === 'vmcp-1' ? 'Issue Tracker vMCP' : `vMCP ${id.slice(5)}`,
			components: [
				createVMCPComponent(componentEntry, {
					id: 'component-github',
					toolPrefix: 'github_',
					...(overrides ? { toolOverrides: overrides } : {})
				})
			]
		},
		[componentEntry]
	);
}

function createRemoteVMcp(
	overrides: ToolOverride[] = remoteToolOverrides,
	toolPreview = remoteToolPreview
) {
	const entry = createMCPCatalogEntry({
		id: 'entry-remote',
		name: 'Remote Records',
		runtime: 'remote',
		manifest: { toolPreview }
	});

	return createVMCP(
		{
			id: 'vmcp-remote',
			displayName: 'Remote Records vMCP',
			components: [
				createVMCPComponent(entry, {
					id: 'component-remote',
					toolPrefix: 'remote_',
					toolOverrides: overrides
				})
			]
		},
		[entry]
	);
}

async function renderVMcpsPage(
	vmcp: VMCPTestResource,
	extraEntries: ReturnType<typeof createMCPCatalogEntry>[] = [],
	extraVMcps: VMCPTestResource[] = [],
	catalogEntries: ReturnType<typeof createMCPCatalogEntry>[] = [componentEntry, ...extraEntries]
) {
	worker.use(
		http.get('/api/vmcps', () => HttpResponse.json({ items: [vmcp, ...extraVMcps] })),
		http.get(`/api/vmcps/${vmcp.id}`, () => HttpResponse.json(vmcp))
	);
	mcpServersAndEntries.current = {
		entries: catalogEntries,
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	await preparePageData();
	const rendered = render(VMcpsPage);
	await expect
		.element(page.getByRole('button', { name: `Edit ${vmcp.displayName}` }))
		.toBeVisible();
	return rendered;
}

async function expandServers(name = 'Issue Tracker vMCP', count = 1) {
	const label = count === 1 ? 'server' : 'servers';
	await page.getByRole('button', { name: `Show ${count} ${label} in ${name}` }).click();
	await tick();
}

function componentBlock() {
	return page.getByRole('button', { name: componentEntry.manifest.name!, exact: true });
}

function editorRefreshButton() {
	const editorDialog = page.getByCSS('dialog[open]').filter({
		has: page.getByRole('button', { name: 'Confirm', exact: true })
	});
	return editorDialog.getByRole('button', { name: 'Refresh tools', exact: true });
}

function mockUpdateEntry(vmcp: VMCPTestResource, onUpdate: (manifest: unknown) => void) {
	worker.use(
		http.get(`/api/vmcps/${vmcp.id}`, () => HttpResponse.json(vmcp)),
		http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) => {
			const manifest = (await request.json()) as Record<string, unknown>;
			onUpdate(manifest);
			return HttpResponse.json({ ...vmcp, ...manifest });
		})
	);
}

function componentServersFrom(manifest: unknown) {
	return (manifest as { components?: VMCPTestResource['components'] }).components ?? [];
}

function dispatchVisiblePage() {
	const descriptor = Object.getOwnPropertyDescriptor(document, 'visibilityState');
	if (document.visibilityState !== 'visible') {
		Object.defineProperty(document, 'visibilityState', {
			configurable: true,
			value: 'visible'
		});
	}
	document.dispatchEvent(new Event('visibilitychange'));

	return () => {
		if (descriptor) {
			Object.defineProperty(document, 'visibilityState', descriptor);
		} else {
			Reflect.deleteProperty(document, 'visibilityState');
		}
	};
}

async function clickButton(locator: ReturnType<typeof page.getByRole>) {
	const element = await locator.element();
	if (!(element instanceof HTMLElement)) throw new Error('Expected a button element');
	element.click();
}

function mockEntryDetails(entry: MCPCatalogEntry) {
	const listServers = vi.fn();
	worker.use(
		http.get(`/api/mcp-catalogs/default/entries/${entry.id}`, () => HttpResponse.json(entry)),
		http.get(`/api/mcp-catalogs/default/entries/${entry.id}/servers`, () => {
			listServers();
			return HttpResponse.json({ items: [] });
		})
	);
	return listServers;
}

function panelCard(name: string) {
	return page.getByRole('button', { name: new RegExp(`View ${name} details`) });
}

function centerOf(el: Element) {
	const rect = el.getBoundingClientRect();
	return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
}

type Point = { x: number; y: number };

function pointer(el: HTMLElement, type: string, pointerId: number, at: Point) {
	el.dispatchEvent(
		new PointerEvent(type, {
			bubbles: true,
			cancelable: true,
			button: 0,
			pointerId,
			clientX: at.x,
			clientY: at.y
		})
	);
}

async function pressCard(locator: ReturnType<typeof page.getByRole>, pointerId: number) {
	const el = await locator.element();
	if (!(el instanceof HTMLElement)) throw new Error('Expected an HTMLElement');
	// A synthesized pointerId is not a live pointer, so real capture would throw.
	el.setPointerCapture = () => {};

	const from = centerOf(el);
	pointer(el, 'pointerdown', pointerId, from);
	return { el, from };
}

describe('vMCPs Page', () => {
	describe('first-class vMCP listing', () => {
		it('loads vMCPs independently of composite entries in the catalog store', async () => {
			await renderVMcpsPage(createVMcp(), [], [], []);

			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.toBeVisible();
			await expect.element(componentBlock()).toBeVisible();
		});

		it('keeps the personal vMCP filter available when it has no matches', async () => {
			await renderVMcpsPage(createVMcp());

			const personalOnly = page.getByRole('checkbox', { name: 'Show only my vMCPs' });
			await personalOnly.click();
			await expect.element(personalOnly).toBeChecked();
			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.not.toBeInTheDocument();

			await personalOnly.click();
			await expect.element(personalOnly).not.toBeChecked();
			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.toBeVisible();
		});
	});

	describe('component with stored tool overrides', () => {
		it('uses the cached snapshot preview for persisted tools', async () => {
			const legacyPreview = vi.fn();
			worker.use(
				http.post(
					`/api/mcp-catalogs/default/entries/${'vmcp-1'}/component-github/generate-tool-previews`,
					() => {
						legacyPreview();
						return HttpResponse.json({});
					}
				)
			);

			await renderVMcpsPage(createVMcp(toolOverrides));
			await componentBlock().click();

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			await expect.element(page.getByText('create_issue').first()).toBeVisible();
			await expect.element(page.getByText('list_issues').first()).toBeVisible();
			await expect.element(page.getByCSS('#edit-tool-component-github-create_issue')).toBeVisible();
			expect(legacyPreview).not.toHaveBeenCalled();
		});

		it('edits the stored overrides instead of running the tool setup flow', async () => {
			await renderVMcpsPage(createVMcp(toolOverrides));

			await componentBlock().click();

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			await expect.element(page.getByText('create_issue').first()).toBeVisible();
			await expect.element(page.getByText('list_issues').first()).toBeVisible();
			// The prefix field has no accessible name, and the setup flow keeps a second copy of
			// the same editor mounted, so scope the lookup to the dialog that is open.
			await expect
				.element(page.getByCSS('dialog[open] input[placeholder="No prefix"]'))
				.toHaveValue('github_');
			await expect.element(page.getByRole('button', { name: 'Delete MCP Server' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Refresh tools' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.not.toBeInTheDocument();
		});

		it('keeps omitted and explicitly disabled saved tools disabled when reopened', async () => {
			const savedOverrides: ToolOverride[] = [
				{ name: 'create_issue', description: 'Create an issue', enabled: true },
				{ name: 'list_issues', description: 'List issues' },
				{ name: 'other_tool', description: 'Other tool', enabled: false }
			];
			const vmcp = createVMcp(savedOverrides);
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);

			await renderVMcpsPage(vmcp);
			await componentBlock().click();

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			const enabledTools = page.getByRole('checkbox', { name: 'Enabled' });
			await expect.element(enabledTools.nth(0)).toBeChecked();
			await expect.element(enabledTools.nth(1)).not.toBeChecked();
			await expect.element(enabledTools.nth(2)).not.toBeChecked();

			await page.getByRole('button', { name: 'Confirm', exact: true }).click();
			await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
			const updatedComponent = componentServersFrom(update.mock.calls[0][0])[0];
			expect(updatedComponent.toolOverrides).toEqual(
				expect.arrayContaining([
					expect.objectContaining({ name: 'create_issue', enabled: true }),
					expect.objectContaining({ name: 'list_issues', enabled: false }),
					expect.objectContaining({ name: 'other_tool', enabled: false })
				])
			);

			await expect.element(page.getByCSS('dialog[open]')).not.toBeInTheDocument();
			await componentBlock().click();
			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			const reopenedEnabledTools = page.getByRole('checkbox', { name: 'Enabled' });
			await expect.element(reopenedEnabledTools.nth(0)).toBeChecked();
			await expect.element(reopenedEnabledTools.nth(1)).not.toBeChecked();
			await expect.element(reopenedEnabledTools.nth(2)).not.toBeChecked();
		});

		it('saves the edited overrides back onto the vMCP', async () => {
			const vmcp = createVMcp(toolOverrides);
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);

			await renderVMcpsPage(vmcp);
			await componentBlock().click();

			// Enable the tool that is currently excluded from the composite.
			await page.getByRole('checkbox', { name: 'Enabled' }).nth(1).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0])[0]).toMatchObject({
				id: 'component-github',
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: componentEntry.id,
				toolPrefix: 'github_',
				toolOverrides: [
					{ name: 'create_issue', enabled: true },
					{ name: 'list_issues', enabled: true }
				]
			});
		});

		it('refreshes persisted tools through the vMCP endpoint without querying a source server', async () => {
			const vmcp = createVMcp(toolOverrides);
			const refreshPreview = vi.fn();
			const legacyPreview = vi.fn();
			worker.use(
				http.post(
					`/api/vmcps/${vmcp.id}/components/component-github/generate-tool-previews`,
					() => {
						refreshPreview();
						return HttpResponse.json(componentEntry);
					}
				),
				http.post(
					`/api/mcp-catalogs/default/entries/${vmcp.id}/component-github/generate-tool-previews`,
					() => {
						legacyPreview();
						return HttpResponse.json({});
					}
				)
			);

			await renderVMcpsPage(vmcp);

			await componentBlock().click();
			const refreshButton = editorRefreshButton();
			await expect.element(refreshButton).toBeVisible();
			await clickButton(refreshButton);

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			expect(refreshPreview).toHaveBeenCalledOnce();
			expect(legacyPreview).not.toHaveBeenCalled();
		});
	});

	describe('component without stored tool overrides', () => {
		it('offers modifying tools or deleting the server', async () => {
			await renderVMcpsPage(createVMcp());

			await componentBlock().click();

			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Delete MCP Server' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.not.toBeInTheDocument();
		});

		it('starts the tool setup flow from Modify Tools', async () => {
			await renderVMcpsPage(createVMcp());

			await componentBlock().click();
			await page.getByRole('button', { name: 'Modify Tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.toBeVisible();
		});

		it('uses flattened catalog configuration for tool previews', async () => {
			const entry = createMCPCatalogEntry({
				id: 'entry-configured',
				name: 'Configured Server',
				manifest: {
					config: [
						{
							key: 'TOKEN',
							name: 'Token',
							description: 'API token',
							required: true,
							sensitive: true,
							value: '',
							usage: 'env'
						},
						{
							key: 'Authorization',
							name: 'Authorization',
							description: 'Authorization header',
							required: true,
							sensitive: true,
							value: '',
							usage: 'header'
						}
					]
				}
			});
			const vmcp = createVMCP(
				{
					id: 'vmcp-configured',
					displayName: 'Configured vMCP',
					components: [createVMCPComponent(entry, { id: 'component-configured' })]
				},
				[entry]
			);
			const generatedTools = [
				{ id: 'configured_tool', name: 'configured_tool', description: 'Uses configuration' }
			];
			worker.use(
				http.post(
					`/api/vmcps/${vmcp.id}/components/component-configured/generate-tool-previews`,
					() =>
						HttpResponse.json({
							...entry,
							manifest: { ...entry.manifest, toolPreview: generatedTools }
						})
				)
			);

			await renderVMcpsPage(vmcp, [entry], [], [entry]);
			await page.getByRole('button', { name: 'Configured Server', exact: true }).click();
			await page.getByRole('button', { name: 'Modify Tools' }).click();
			await page.getByRole('button', { name: 'Configure Tools', exact: true }).click();

			await expect.element(page.getByText('configured_tool', { exact: true })).toBeVisible();
		});

		it('removes the server from the vMCP without visiting the setup flow', async () => {
			const vmcp = createVMcp();
			const secondEntry = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });
			vmcp.components.push(createVMCPComponent(secondEntry, { id: 'component-slack' }));
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);

			await renderVMcpsPage(vmcp);
			await componentBlock().click();
			await page.getByRole('button', { name: 'Delete MCP Server' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0])).toHaveLength(1);
			expect(componentServersFrom(update.mock.calls[0][0])[0]).toMatchObject({
				id: 'component-slack',
				mcpServerCatalogEntryID: secondEntry.id
			});
		});

		describe('remote component tool refresh', () => {
			it('prompts for OAuth and retries preview generation without widening saved tools', async () => {
				const vmcp = createRemoteVMcp();
				const generatedTools = [
					...remoteToolPreview,
					{
						id: 'new_tool',
						name: 'new_tool',
						description: 'A newly discovered tool'
					}
				];
				const generatePreview = vi.fn();
				const oauthURL = 'https://remote.example/oauth/authorize';

				worker.use(
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews`,
						() => {
							generatePreview();
							if (generatePreview.mock.calls.length === 1) {
								return HttpResponse.json(
									{ error: 'MCP server requires OAuth authentication' },
									{ status: 400 }
								);
							}
							return HttpResponse.json({
								...remoteEntry,
								manifest: {
									...remoteEntry.manifest,
									toolPreview: generatedTools
								}
							});
						}
					),
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews/oauth-url`,
						() => HttpResponse.json({ oauthURL })
					)
				);

				await renderVMcpsPage(vmcp, [], [], [remoteEntry]);
				await page.getByRole('button', { name: 'Remote Records', exact: true }).click();
				await expect
					.element(page.getByRole('heading', { name: 'Configure Remote Records Tools' }))
					.toBeVisible();
				const refreshButton = editorRefreshButton();
				await expect.element(refreshButton).toBeVisible();
				await clickButton(refreshButton);

				await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
				await expect
					.element(page.getByRole('link', { name: 'Authenticate' }))
					.toHaveAttribute('href', oauthURL);
				expect(generatePreview).toHaveBeenCalledOnce();

				const restoreVisibility = dispatchVisiblePage();
				try {
					await expect
						.element(page.getByRole('heading', { name: 'Configure Remote Records Tools' }))
						.toBeVisible();
					await expect.element(page.getByText('new_tool').first()).toBeVisible();
					expect(generatePreview).toHaveBeenCalledTimes(2);

					const enabledTools = page.getByRole('checkbox', { name: 'Enabled' });
					await expect.element(enabledTools.nth(0)).toBeChecked();
					await expect.element(enabledTools.nth(1)).not.toBeChecked();
					await expect.element(enabledTools.nth(2)).not.toBeChecked();
					await expect
						.element(page.getByCSS('dialog[open] input[placeholder="No prefix"]'))
						.toHaveValue('remote_');
					await expect.element(page.getByText('find_records').first()).toBeVisible();
				} finally {
					restoreVisibility();
				}
			});

			it('enables all tools from an initial remote preview without saved overrides', async () => {
				const vmcp = createRemoteVMcp([], []);
				const generatedTools = [
					...remoteToolPreview,
					{ id: 'new_tool', name: 'new_tool', description: 'A newly discovered tool' }
				];
				const generatePreview = vi.fn();

				worker.use(
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews`,
						() => {
							generatePreview();
							return HttpResponse.json({
								...remoteEntry,
								manifest: { ...remoteEntry.manifest, toolPreview: generatedTools }
							});
						}
					),
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews/oauth-url`,
						() => HttpResponse.json({ oauthURL: 'https://remote.example/oauth/authorize' })
					)
				);

				await renderVMcpsPage(vmcp, [], [], [remoteEntry]);
				await page.getByRole('button', { name: 'Remote Records', exact: true }).click();
				await page.getByRole('button', { name: 'Modify Tools' }).click();
				const configureButton = page.getByRole('button', {
					name: 'Configure Tools',
					exact: true
				});
				await expect.element(configureButton).toBeVisible();
				await clickButton(configureButton);

				await expect
					.element(page.getByRole('heading', { name: 'Configure Remote Records Tools' }))
					.toBeVisible();
				await expect.element(page.getByText('new_tool').first()).toBeVisible();
				expect(generatePreview).toHaveBeenCalledOnce();

				const enabledTools = page.getByRole('checkbox', { name: 'Enabled' });
				await expect.element(enabledTools.nth(0)).toBeChecked();
				await expect.element(enabledTools.nth(1)).toBeChecked();
				await expect.element(enabledTools.nth(2)).toBeChecked();
			});

			it('does not open the editor when the OAuth URL cannot be fetched', async () => {
				const vmcp = createRemoteVMcp();
				const getOauthURL = vi.fn();

				worker.use(
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews`,
						() =>
							HttpResponse.json(
								{ error: 'MCP server requires OAuth authentication' },
								{ status: 400 }
							)
					),
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews/oauth-url`,
						() => {
							getOauthURL();
							return HttpResponse.json({ error: 'OAuth URL unavailable' }, { status: 500 });
						}
					)
				);

				await renderVMcpsPage(vmcp, [], [], [remoteEntry]);
				await page.getByRole('button', { name: 'Remote Records', exact: true }).click();
				const refreshButton = editorRefreshButton();
				await expect.element(refreshButton).toBeVisible();
				await clickButton(refreshButton);

				await vi.waitFor(() => expect(getOauthURL).toHaveBeenCalledOnce());
				await expect
					.element(page.getByRole('button', { name: 'Confirm', exact: true }))
					.not.toBeInTheDocument();
				await expect
					.element(page.getByRole('link', { name: 'Authenticate' }))
					.not.toBeInTheDocument();
			});

			it('does not reopen the editor after cancellation while OAuth preview is pending', async () => {
				const vmcp = createRemoteVMcp();
				let previewCalls = 0;
				let resolveRetry: ((response: Response) => void) | undefined;

				worker.use(
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews`,
						() => {
							previewCalls += 1;
							if (previewCalls === 1) {
								return HttpResponse.json(
									{ error: 'MCP server requires OAuth authentication' },
									{ status: 400 }
								);
							}
							return new Promise<Response>((resolve) => {
								resolveRetry = resolve;
							});
						}
					),
					http.post(
						`/api/vmcps/${vmcp.id}/components/component-remote/generate-tool-previews/oauth-url`,
						() =>
							HttpResponse.json({
								oauthURL: 'https://remote.example/oauth/authorize'
							})
					)
				);

				await renderVMcpsPage(vmcp, [], [], [remoteEntry]);
				await page.getByRole('button', { name: 'Remote Records', exact: true }).click();
				const refreshButton = editorRefreshButton();
				await expect.element(refreshButton).toBeVisible();
				await clickButton(refreshButton);
				await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();

				await page.getByRole('button', { name: 'Retry', exact: true }).click();
				try {
					await vi.waitFor(() => {
						expect(previewCalls).toBe(2);
						expect(resolveRetry).toBeDefined();
					});

					const openDialog = page.getByCSS('dialog[open]').first();
					const dialogElement = await openDialog.element();
					if (!(dialogElement instanceof HTMLDialogElement)) {
						throw new Error('Expected the setup dialog to be open');
					}
					dialogElement.close();
					await expect.element(page.getByCSS('dialog[open]')).not.toBeInTheDocument();

					resolveRetry?.(
						new Response(
							JSON.stringify({
								...remoteEntry,
								manifest: {
									...remoteEntry.manifest,
									toolPreview: remoteToolPreview
								}
							}),
							{ status: 200, headers: { 'Content-Type': 'application/json' } }
						)
					);
					await new Promise((resolve) => setTimeout(resolve, 0));
					expect(previewCalls).toBe(2);
					await expect.element(page.getByCSS('dialog[open]')).not.toBeInTheDocument();
				} finally {
					resolveRetry?.(new Response('{}', { status: 200 }));
				}
			});
		});
	});

	describe('dragging a server from the panel onto the canvas', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });
		let listSlackServers: ReturnType<typeof mockEntryDetails>;

		beforeEach(() => {
			listSlackServers = mockEntryDetails(slack);
		});

		function vmcpCard() {
			return page.getByRole('button', { name: 'Edit Issue Tracker vMCP' });
		}

		/** Presses the Slack panel card and drags it over the vMCP card, without releasing. */
		async function dragSlackOntoVMcp(pointerId: number) {
			const target = await vmcpCard().element();
			const { el } = await pressCard(panelCard('Slack'), pointerId);
			const to = centerOf(target);
			pointer(el, 'pointermove', pointerId, to);
			await tick();
			return { el, to };
		}

		it('marks both the dragged card and the vMCP it is linked to', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);

			await dragSlackOntoVMcp(12);

			// The panel reads the drag state and the canvas reads the link state, so both
			// updating proves they share one source of truth. The drag ghost carries the same
			// class, so scope the canvas assertion to the vMCP card itself.
			await expect.element(panelCard('Slack')).toHaveClass(/opacity-30/);
			await expect.element(page.getByCSS('.vmcp-drop-target').first()).toBeInTheDocument();
		});

		it('adds the dropped server to the vMCP it landed on', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);

			const { el, to } = await dragSlackOntoVMcp(13);
			pointer(el, 'pointerup', 13, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				name: slack.manifest.name,
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: slack.id,
				catalogEntry: {
					manifest: expect.objectContaining({
						name: slack.manifest.name,
						runtime: slack.manifest.runtime
					}),
					unsupportedTools: []
				}
			});
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).not.toHaveProperty(
				'mcpServerID'
			);
		});

		it('generates a source preview and stores it in the new component snapshot', async () => {
			const vmcp = createVMcp();
			const generatedTools = [
				{ id: 'send_message', name: 'send_message', description: 'Send a message' }
			];
			const generated = {
				...slack,
				unsupportedTools: ['unsupported_tool'],
				manifest: { ...slack.manifest, toolPreview: generatedTools }
			};
			const generatePreview = vi.fn();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			worker.use(
				http.post(
					`/api/mcp-catalogs/default/entries/${slack.id}/generate-tool-previews`,
					({ request }) => {
						generatePreview(new URL(request.url));
						return HttpResponse.json(generated);
					}
				)
			);

			// Render after registering the source-generation handler so the drag starts from a
			// catalog entry without a preview and must use the source entry's endpoint.
			await renderVMcpsPage(vmcp, [slack]);
			const { el, to } = await dragSlackOntoVMcp(16);
			pointer(el, 'pointerup', 16, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(generatePreview).toHaveBeenCalledOnce();
			expect(generatePreview.mock.calls[0][0].searchParams.get('dryRun')).toBe('true');
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				name: slack.manifest.name,
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: slack.id,
				catalogEntry: {
					manifest: {
						name: slack.manifest.name,
						runtime: slack.manifest.runtime,
						toolPreview: generatedTools
					},
					unsupportedTools: ['unsupported_tool']
				}
			});
		});

		it('leaves the vMCP alone when Escape cancels the drag before release', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);

			const { el, to } = await dragSlackOntoVMcp(14);
			await userEvent.keyboard('{Escape}');
			pointer(el, 'pointerup', 14, to);
			await tick();

			expect(update).not.toHaveBeenCalled();
			await expect.element(page.getByCSS('.vmcp-drop-target')).not.toBeInTheDocument();
		});

		it('opens the server details when the press never travels far enough to drag', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);

			const { el, from } = await pressCard(panelCard('Slack'), 15);
			pointer(el, 'pointerup', 15, from);

			await expect.element(page.getByRole('dialog').first()).toBeVisible();
			await vi.waitFor(() => expect(listSlackServers).toHaveBeenCalled());
		});
	});

	describe('dragging a server from the panel onto the table view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		beforeEach(() => {
			mockEntryDetails(slack);
		});

		async function showTableView() {
			await page.getByRole('button', { name: 'Table View' }).click({ force: true });
			await expect.element(vmcpRow()).toBeVisible();
		}

		function vmcpRow() {
			return page.getByRole('cell', { name: 'Issue Tracker vMCP' });
		}

		function dropZone() {
			return page.getByRole('region', { name: 'MCP Servers in Issue Tracker vMCP' });
		}

		function openDialog() {
			return page.getByCSS('dialog[open]');
		}

		/** Presses the Slack panel card and drags it over `target`, without releasing. */
		async function dragSlackOnto(target: ReturnType<typeof page.getByRole>, pointerId: number) {
			const targetEl = await target.element();
			const { el } = await pressCard(panelCard('Slack'), pointerId);
			const to = centerOf(targetEl);
			pointer(el, 'pointermove', pointerId, to);
			await tick();
			return { el, to };
		}

		it('marks the row the drag is linked to', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();

			await dragSlackOnto(vmcpRow(), 20);

			await expect.element(page.getByCSS('tbody tr').first()).toHaveClass(/outline-primary/);
		});

		it('adds the dropped server to the row it landed on', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);
			await showTableView();

			const { el, to } = await dragSlackOnto(vmcpRow(), 21);
			pointer(el, 'pointerup', 21, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				name: slack.manifest.name,
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: slack.id,
				catalogEntry: {
					manifest: expect.objectContaining({ name: slack.manifest.name }),
					unsupportedTools: []
				}
			});
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).not.toHaveProperty(
				'mcpServerID'
			);
		});

		it('lists the servers of the row that was clicked', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();

			await vmcpRow().click();

			await expect.element(dropZone()).toBeVisible();
			await expect
				.element(
					page.getByCSS('dialog[open]').getByText(componentEntry.manifest.name!, { exact: true })
				)
				.toBeVisible();
			await expect.element(openDialog().getByRole('button', { name: 'Edit tools' })).toBeVisible();
			await expect.element(openDialog().getByRole('button', { name: 'Remove' })).toBeVisible();
		});

		it('opens the tool setup flow from Edit tools when no overrides are stored', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();

			await openDialog().getByRole('button', { name: 'Edit tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
		});

		it('opens the stored overrides editor from Edit tools', async () => {
			await renderVMcpsPage(createVMcp(toolOverrides), [slack]);
			await showTableView();
			await vmcpRow().click();

			await openDialog().getByRole('button', { name: 'Edit tools' }).click();

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
		});

		it('prompts to remove the server without an extra choice dialog', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();

			await openDialog().getByRole('button', { name: 'Remove' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Cancel' }).click();
			await expect.element(page.getByText('Confirm Remove')).not.toBeVisible();
		});

		it('paints the drag ghost above the open dialog', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();
			await expect.element(dropZone()).toBeVisible();

			await dragSlackOnto(dropZone(), 23);

			const overlay = await page.getByCSS('[data-vmcp-drag-overlay]').element();
			const dialog = await page.getByCSS('dialog[open]').element();
			expect(Number(getComputedStyle(overlay).zIndex)).toBeGreaterThan(
				Number(getComputedStyle(dialog).zIndex)
			);
		});

		it('adds the dropped server to the vMCP whose dialog is open', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);
			await showTableView();
			await vmcpRow().click();
			await expect.element(dropZone()).toBeVisible();

			const { el, to } = await dragSlackOnto(dropZone(), 22);
			pointer(el, 'pointerup', 22, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				name: slack.manifest.name,
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: slack.id,
				catalogEntry: {
					manifest: expect.objectContaining({ name: slack.manifest.name }),
					unsupportedTools: []
				}
			});
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).not.toHaveProperty(
				'mcpServerID'
			);
		});
	});

	describe('vMCPs graphs view', () => {
		it('expands the first five connectors by default', async () => {
			await renderVMcpsPage(createVMcp());

			await expect.element(componentBlock()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toHaveAttribute('aria-expanded', 'true');
		});

		it('lets the user expand more than one connector', async () => {
			const extras = [2, 3, 4, 5, 6].map((n) => createVMcp(undefined, `vmcp-${n}`));
			await renderVMcpsPage(createVMcp(), [], extras);

			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 2' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 3' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 4' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 5' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Show 1 server in vMCP 6' }))
				.toBeVisible();

			await expandServers('vMCP 6');

			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 6' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toBeVisible();
		});

		it('zooms the world from the toolbar', async () => {
			await renderVMcpsPage(createVMcp());
			const world = page.getByCSS('[data-vmcp-world]');
			await expect.element(world).toBeInTheDocument();
			const before = (await world.element()).getAttribute('style') ?? '';

			await page.getByRole('button', { name: 'Zoom in' }).click();
			await tick();

			const after = (await world.element()).getAttribute('style') ?? '';
			expect(after).not.toBe(before);
			expect(after).toContain('scale(');
		});

		it('does not pan when dragging from a vMCP card', async () => {
			await renderVMcpsPage(createVMcp());
			const world = await page.getByCSS('[data-vmcp-world]').element();
			const before = world.getAttribute('style');
			const card = await page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }).element();
			card.dispatchEvent(
				new PointerEvent('pointerdown', {
					bubbles: true,
					cancelable: true,
					clientX: 40,
					clientY: 40,
					button: 0,
					pointerId: 7
				})
			);
			card.dispatchEvent(
				new PointerEvent('pointermove', {
					bubbles: true,
					cancelable: true,
					clientX: 120,
					clientY: 90,
					button: 0,
					pointerId: 7
				})
			);
			await tick();
			expect(world.getAttribute('style')).toBe(before);
		});
	});
});
