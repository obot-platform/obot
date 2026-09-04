import type { AccessControlRule } from '$lib/services';
import type { HostedAgentAccessPolicy, ModelAccessPolicy, SkillAccessPolicy } from '$lib/services';
import { createMCPCatalogEntry, createMCPCatalogServer } from '../../tests/helpers/mcp';
import { renderOpenDialog } from '../../tests/helpers/openDialog';
import { worker } from '../../tests/mocks/worker';
import CurrentAccessDialog from './CurrentAccessDialog.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';

const userTarget = {
	kind: 'user' as const,
	id: 'user-1',
	name: 'Ada Lovelace',
	groupIds: ['engineering']
};

const groupTarget = {
	kind: 'group' as const,
	id: 'engineering',
	name: 'Engineering'
};

// The MCP store caches across tests in this file, so every test resolves MCP resources from the
// same catalog fixtures.
const catalogEntry = createMCPCatalogEntry({ id: 'entry-1', name: 'GitHub' });
const catalogServer = createMCPCatalogServer({ id: 'server-1', name: 'Slack', userID: 'user-1' });

function mcpRule(
	overrides: Partial<AccessControlRule> & { id: string; displayName: string }
): AccessControlRule {
	return {
		created: '2026-01-01T00:00:00Z',
		subjects: [],
		resources: [],
		...overrides
	};
}

function modelPolicy(
	overrides: Partial<ModelAccessPolicy> & { id: string; displayName: string }
): ModelAccessPolicy {
	return {
		created: '2026-01-01T00:00:00Z',
		subjects: [],
		models: [],
		...overrides
	};
}

function skillPolicy(
	overrides: Partial<SkillAccessPolicy> & { id: string; displayName: string }
): SkillAccessPolicy {
	return {
		created: '2026-01-01T00:00:00Z',
		subjects: [],
		resources: [],
		...overrides
	};
}

function hostedPolicy(
	overrides: Partial<HostedAgentAccessPolicy> & { id: string; displayName: string }
): HostedAgentAccessPolicy {
	return {
		created: '2026-01-01T00:00:00Z',
		subjects: [],
		resources: [],
		...overrides
	};
}

function mockResources() {
	worker.use(
		http.get('/api/all-mcps/entries', () => HttpResponse.json({ items: [catalogEntry] })),
		http.get('/api/all-mcps/servers', () => HttpResponse.json({ items: [catalogServer] })),
		http.get('/api/mcp-servers', () => HttpResponse.json({ items: [] })),
		http.get('/api/mcp-server-instances', () => HttpResponse.json({ items: [] })),
		http.get('/api/models', () =>
			HttpResponse.json({ items: [{ id: 'model-1', name: 'gpt-5', displayName: 'GPT-5' }] })
		),
		http.get('/api/skills', () =>
			HttpResponse.json({ items: [{ id: 'skill-1', name: 'Summarize' }] })
		),
		http.get('/api/skill-repositories', () =>
			HttpResponse.json({ items: [{ id: 'repo-1', displayName: 'Internal Skills' }] })
		),
		http.get('/api/hosted-agents', () =>
			HttpResponse.json({ items: [{ id: 'agent-1', name: 'Support Bot' }] })
		),
		http.get('/api/users', () =>
			HttpResponse.json({
				items: [
					{
						id: 'owner-1',
						username: 'owner-1',
						email: 'owner@example.com',
						displayName: 'Pat Power',
						created: '2026-01-01T00:00:00Z',
						role: 1,
						effectiveRole: 1,
						explicitRole: true,
						groups: []
					}
				]
			})
		)
	);
}

function mockPolicies({
	mcp = [] as AccessControlRule[],
	workspaceMcp = [] as AccessControlRule[],
	models = [] as ModelAccessPolicy[],
	skills = [] as SkillAccessPolicy[],
	hosted = [] as HostedAgentAccessPolicy[]
} = {}) {
	worker.use(
		http.get('/api/mcp-catalogs/default/access-control-rules', () =>
			HttpResponse.json({ items: mcp })
		),
		http.get('/api/workspaces/all-access-control-rules', () =>
			HttpResponse.json({ items: workspaceMcp })
		),
		http.get('/api/model-access-policies', () => HttpResponse.json({ items: models })),
		http.get('/api/skill-access-rules', () => HttpResponse.json({ items: skills })),
		http.get('/api/hosted-agent-access-rules', () => HttpResponse.json({ items: hosted }))
	);
}

describe('CurrentAccessDialog.svelte', () => {
	beforeEach(() => {
		mockResources();
	});

	it('lists the MCP resources granted by policies that apply to the user', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'mcp-user',
					displayName: 'Direct MCP',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				}),
				mcpRule({
					id: 'mcp-other',
					displayName: 'Someone Else MCP',
					subjects: [{ type: 'user', id: 'user-2' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			],
			workspaceMcp: [
				mcpRule({
					id: 'mcp-ws',
					displayName: 'Workspace MCP',
					powerUserWorkspaceID: 'ws-9',
					subjects: [{ type: 'group', id: 'engineering' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });

		await expect
			.element(dialog.getByText('Ada Lovelace | Access Policies', { exact: true }))
			.toBeVisible();
		await expect.element(dialog.getByText('GitHub', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText('Slack', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText(/Catalog Entry · Granted by/)).toBeVisible();
		await expect.element(dialog.getByText(/MCP Server · Granted by/)).toBeVisible();

		// Each resource links back to the policies that grant it.
		await expect
			.element(dialog.getByRole('link', { name: 'Direct MCP' }))
			.toHaveAttribute('href', '/mcp-servers/access-policies/mcp-user');
		await expect
			.element(dialog.getByRole('link', { name: 'Workspace MCP' }))
			.toHaveAttribute('href', '/mcp-servers/access-policies/w/ws-9/r/mcp-ws');

		// The rule assigned to another user contributes neither its resource nor a link.
		await expect
			.element(dialog.getByText('Someone Else MCP', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('filters MCP resources as the user types in search', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'mcp-user',
					displayName: 'Direct MCP',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				})
			],
			workspaceMcp: [
				mcpRule({
					id: 'mcp-ws',
					displayName: 'Workspace MCP',
					powerUserID: 'owner-1',
					powerUserWorkspaceID: 'ws-9',
					subjects: [{ type: 'group', id: 'engineering' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		await expect.element(dialog.getByText('GitHub', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText('Slack', { exact: true })).toBeVisible();

		await dialog.getByPlaceholder('Search MCP servers...').fill('git');

		await expect.element(dialog.getByText('GitHub', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText('Slack', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(dialog.getByRole('region', { name: "Pat Power's Registry" }))
			.not.toBeInTheDocument();

		await dialog.getByPlaceholder('Search MCP servers...').fill('no-such-server');
		await expect
			.element(dialog.getByText('No MCP servers match this search.', { exact: true }))
			.toBeVisible();
	});

	it('collapses a section to Everything when a policy grants the wildcard resource', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'mcp-all',
					displayName: 'Everything MCP',
					subjects: [{ type: 'selector', id: '*' }],
					resources: [{ type: 'selector', id: '*' }]
				}),
				mcpRule({
					id: 'mcp-user',
					displayName: 'Direct MCP',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });

		await expect.element(dialog.getByText('Everything', { exact: true })).toBeVisible();
		await expect.element(dialog.getByRole('link', { name: 'Everything MCP' })).toBeVisible();
		await expect.element(dialog.getByText('GitHub', { exact: true })).not.toBeInTheDocument();
	});

	it('applies a global Everything grant only to the global registry', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'global-all',
					displayName: 'Global Everything',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'selector', id: '*' }]
				}),
				mcpRule({
					id: 'global-entry',
					displayName: 'Global GitHub',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				})
			],
			workspaceMcp: [
				mcpRule({
					id: 'owner-server',
					displayName: 'Owner Slack',
					powerUserID: 'owner-1',
					powerUserWorkspaceID: 'ws-owner-1',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		const globalRegistry = dialog.getByRole('region', { name: 'Global Registry' });
		const ownerRegistry = dialog.getByRole('region', { name: "Pat Power's Registry" });

		await expect.element(globalRegistry.getByText('Everything', { exact: true })).toBeVisible();
		await expect
			.element(globalRegistry.getByText('GitHub', { exact: true }))
			.not.toBeInTheDocument();
		await expect.element(ownerRegistry.getByText('Slack', { exact: true })).toBeVisible();
		await expect
			.element(ownerRegistry.getByText('Everything', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('applies a power user Everything grant only to that power user registry', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'global-entry',
					displayName: 'Global GitHub',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				})
			],
			workspaceMcp: [
				mcpRule({
					id: 'owner-all',
					displayName: 'Owner Everything',
					powerUserID: 'owner-1',
					powerUserWorkspaceID: 'ws-owner-1',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'selector', id: '*' }]
				}),
				mcpRule({
					id: 'owner-server',
					displayName: 'Owner Slack',
					powerUserID: 'owner-1',
					powerUserWorkspaceID: 'ws-owner-1',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		const globalRegistry = dialog.getByRole('region', { name: 'Global Registry' });
		const ownerRegistry = dialog.getByRole('region', { name: "Pat Power's Registry" });

		await expect.element(globalRegistry.getByText('GitHub', { exact: true })).toBeVisible();
		await expect
			.element(globalRegistry.getByText('Everything', { exact: true }))
			.not.toBeInTheDocument();
		await expect.element(ownerRegistry.getByText('Everything', { exact: true })).toBeVisible();
		await expect.element(ownerRegistry.getByText('Slack', { exact: true })).not.toBeInTheDocument();
	});

	it('resolves model resources, including aliases, when the models tab is opened', async () => {
		mockPolicies({
			models: [
				modelPolicy({
					id: 'model-everyone',
					displayName: 'Everyone Models',
					subjects: [{ type: 'selector', id: '*' }],
					models: [{ id: 'model-1' }, { id: 'obot://llm' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		await dialog.getByRole('button', { name: 'Models' }).click();

		await expect.element(dialog.getByText('GPT-5', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText('Language Model (Chat)', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('link', { name: 'Everyone Models' }).first())
			.toHaveAttribute('href', '/models/access-policies/model-everyone');
	});

	it('resolves skills and skill repositories when the skills tab is opened', async () => {
		mockPolicies({
			skills: [
				skillPolicy({
					id: 'skill-user',
					displayName: 'Ada Skills',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [
						{ type: 'skill', id: 'skill-1' },
						{ type: 'skillRepository', id: 'repo-1' }
					]
				}),
				skillPolicy({
					id: 'skill-sales',
					displayName: 'Sales Skills',
					subjects: [{ type: 'group', id: 'sales' }],
					resources: [{ type: 'selector', id: '*' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		await dialog.getByRole('button', { name: 'Skills' }).click();

		await expect.element(dialog.getByText('Summarize', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText('Internal Skills', { exact: true })).toBeVisible();
		await expect.element(dialog.getByText(/Skill Repository · Granted by/)).toBeVisible();
		// The wildcard belongs to a policy assigned to another group, so it does not collapse the
		// section.
		await expect.element(dialog.getByText('Everything', { exact: true })).not.toBeInTheDocument();
	});

	it('resolves hosted agents when the hosted agents tab is opened', async () => {
		mockPolicies({
			hosted: [
				hostedPolicy({
					id: 'hosted-everyone',
					displayName: 'Everyone Hosted Agents',
					subjects: [{ type: 'selector', id: '*' }],
					resources: [{ type: 'hostedAgent', id: 'agent-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });
		await dialog.getByRole('button', { name: 'Hosted Agents' }).click();

		await expect.element(dialog.getByText('Support Bot', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('link', { name: 'Everyone Hosted Agents' }))
			.toHaveAttribute('href', '/hosted-agents/access-policies/hosted-everyone');
		await expect.element(dialog.getByText('(All Obot Users)', { exact: true })).toBeVisible();
	});

	it('lists everyone and group-assigned resources for a group', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'mcp-everyone',
					displayName: 'Everyone MCP',
					subjects: [{ type: 'selector', id: '*' }],
					resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
				}),
				mcpRule({
					id: 'mcp-eng',
					displayName: 'Engineering MCP',
					subjects: [{ type: 'group', id: 'engineering' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				}),
				mcpRule({
					id: 'mcp-user',
					displayName: 'User Only MCP',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: [{ type: 'mcpServer', id: 'server-1' }]
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: groupTarget });

		await expect
			.element(dialog.getByText('Engineering | Access Policies', { exact: true }))
			.toBeVisible();
		await expect.element(dialog.getByText('GitHub', { exact: true })).toBeVisible();
		await expect.element(dialog.getByRole('link', { name: 'Everyone MCP' })).toBeVisible();
		await expect.element(dialog.getByRole('link', { name: 'Engineering MCP' })).toBeVisible();
		await expect
			.element(dialog.getByText('User Only MCP', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('explains when applicable policies grant no resources in a section', async () => {
		mockPolicies({
			mcp: [
				mcpRule({
					id: 'mcp-user',
					displayName: 'Direct MCP',
					subjects: [{ type: 'user', id: 'user-1' }],
					resources: []
				})
			]
		});

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });

		await expect
			.element(
				dialog.getByText('No MCP servers are granted through this registry.', { exact: true })
			)
			.toBeVisible();
	});

	it('shows an empty state when no policies apply', async () => {
		mockPolicies();

		const dialog = await renderOpenDialog(CurrentAccessDialog, { target: userTarget });

		await expect
			.element(
				dialog.getByText('No access policies currently apply to this user.', { exact: true })
			)
			.toBeVisible();
	});
});
