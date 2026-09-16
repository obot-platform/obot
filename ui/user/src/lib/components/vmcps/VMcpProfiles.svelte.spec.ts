import type { VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import type { VMCP, VMCPManifest, VMCPProfilePermissions } from '$lib/services';
import { createVMCP, createVMCPComponent, createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import VMcpProfiles from './VMcpProfiles.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createVMcp(id: string, withPreview = true) {
	const github = createMCPCatalogEntry({
		id: 'github',
		name: 'GitHub',
		runtime: 'remote',
		manifest: {
			toolPreview: withPreview
				? [
						{ id: 'issues', name: 'list_issues', description: 'List issues' },
						{ id: 'pulls', name: 'list_pulls', description: 'List pull requests' }
					]
				: []
		}
	});
	return createVMCP(
		{
			id,
			displayName: 'Engineering vMCP',
			components: [createVMCPComponent(github, { id: 'github' })]
		},
		[github]
	);
}

function toolFlowStub(overrides: Partial<VMcpToolFlow> = {}): VMcpToolFlow {
	return {
		collectComponentTools: vi.fn(),
		...overrides
	} as VMcpToolFlow;
}

function mockVMcpSave(vmcp: VMCP) {
	const saved = vi.fn();
	worker.use(
		http.get(`/api/vmcps/${vmcp.id}`, () => HttpResponse.json(vmcp)),
		http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			saved(manifest);
			return HttpResponse.json({ ...vmcp, ...manifest });
		})
	);
	return saved;
}

function savedProfiles(saved: ReturnType<typeof vi.fn>, call = 0) {
	return (saved.mock.calls[call][0] as VMCPManifest).profiles ?? [];
}

async function confirmProfileDelete() {
	await expect.element(page.getByText(/Are you sure you want to delete/)).toBeVisible();
	await page.getByRole('button', { name: "Yes, I'm sure" }).click();
}

async function cancelProfileDelete() {
	await page.getByRole('button', { name: 'Cancel' }).click();
	await expect.element(page.getByText(/Are you sure you want to delete/)).not.toBeVisible();
}

async function clickDeleteProfile(name: string) {
	const deleteButton = page.getByRole('button', { name: `Delete ${name}` });
	await expect.element(deleteButton).toBeEnabled();
	await deleteButton.click();
}

async function expandServerTools() {
	await page.getByRole('button', { name: 'Expand' }).click();
}

function toolSwitch(index = 0) {
	return page.getByRole('switch', { name: /^(Enable|Disable) tool$/ }).nth(index);
}

async function assignEveryone() {
	await page.getByRole('combobox', { name: 'Add identities...' }).click();
	await page.getByRole('button', { name: 'All Obot Users', exact: true }).click();
}

/** A vMCP whose own overrides switch one of the two GitHub tools off. */
function createVMcpWithDisabledTool(id: string) {
	const vmcp = createVMcp(id);
	vmcp.components![0].toolOverrides = [
		{ name: 'list_issues', enabled: true },
		{ name: 'list_pulls', enabled: false }
	];
	return vmcp;
}

describe('VMcpProfiles.svelte', () => {
	it.each<{
		name: string;
		permissions: VMCPProfilePermissions;
		expected?: VMCPProfilePermissions;
		withPreview?: boolean;
	}>([
		{
			name: 'all components override explicit empty tools',
			permissions: {
				allowAllComponents: true,
				allowedComponents: { github: { allowedTools: [] } }
			},
			expected: { allowAllComponents: true }
		},
		{
			name: 'null tools on one component',
			permissions: { allowedComponents: { github: { allowedTools: null } } }
		},
		{
			name: 'omitted tools on one component',
			permissions: { allowedComponents: { github: {} } }
		},
		{
			name: 'explicitly selected tools do not become an all-tools grant',
			permissions: {
				allowedComponents: { github: { allowedTools: ['list_issues', 'list_pulls'] } }
			}
		},
		{
			name: 'empty tool list',
			permissions: { allowedComponents: { github: { allowedTools: [] } } }
		},
		{
			name: 'unselected component without previews',
			permissions: { allowedComponents: {} },
			withPreview: false
		},
		{
			name: 'explicit tools without previews',
			permissions: { allowedComponents: { github: { allowedTools: ['list_issues'] } } },
			withPreview: false
		}
	])(
		'preserves $name when editing a profile name',
		async ({ permissions, expected, withPreview }) => {
			const vmcp = createVMcp('vmcp-profile-permissions', withPreview ?? true);
			vmcp.profiles = [
				{
					name: 'Existing',
					subjects: [{ type: 'selector', id: '*' }],
					vmcpPermissions: permissions
				}
			];
			const saved = mockVMcpSave(vmcp);
			render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });
			await page.getByRole('button', { name: 'Edit Existing' }).click();
			await page.getByLabelText('Name').fill('Renamed');
			await page.getByRole('button', { name: 'Save changes' }).click();
			await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
			expect(savedProfiles(saved)[0]).toEqual({
				name: 'Renamed',
				subjects: [{ type: 'selector', id: '*' }],
				vmcpPermissions: expected ?? permissions
			});
		}
	);

	it('saves a profile with its per-server tool grants onto the vMCP', async () => {
		const vmcp = createVMcp('vmcp-create-profile');
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expect.element(page.getByRole('heading', { name: 'Create profile' })).toBeVisible();

		await page.getByLabelText('Name').fill('Support engineers');
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
		await expect.element(page.getByText('list_issues')).not.toBeInTheDocument();
		await expandServerTools();
		await expect.element(page.getByText('list_issues')).toBeVisible();
		await expect.element(page.getByText('list_pulls')).toBeVisible();
		await toolSwitch(1).click();
		await expect.element(page.getByText('1 of 2 tools')).toBeVisible();
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await expect
			.element(page.getByRole('button', { name: 'Edit Support engineers' }))
			.toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)).toEqual([
			{
				name: 'default',
				subjects: [{ type: 'selector', id: '*' }],
				vmcpPermissions: { allowAllComponents: true }
			},
			{
				name: 'Support engineers',
				subjects: [{ type: 'selector', id: '*' }],
				vmcpPermissions: { allowedComponents: { github: { allowedTools: ['list_issues'] } } }
			}
		]);
	});

	it('edits and deletes an existing profile', async () => {
		const vmcp = createVMcp('vmcp-edit-profile');
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Developers');
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await expect.element(page.getByRole('button', { name: 'Edit Developers' })).toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(1));
		saved.mockClear();

		await page.getByRole('button', { name: 'Edit Developers' }).click();
		await page.getByLabelText('Name').fill('Platform developers');
		await page.getByRole('button', { name: 'Save changes' }).click();

		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(1));

		await clickDeleteProfile('Platform developers');
		await cancelProfileDelete();
		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.toBeVisible();

		await clickDeleteProfile('Platform developers');
		await confirmProfileDelete();

		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit default' })).toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
		expect(savedProfiles(saved, 0).map((profile) => profile.name)).toEqual([
			'default',
			'Platform developers'
		]);
		expect(savedProfiles(saved, 1).map((profile) => profile.name)).toEqual(['default']);
	});

	it('deletes an existing profile from the editor', async () => {
		const vmcp = createVMcp('vmcp-delete-from-editor');
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expect.element(page.getByRole('button', { name: /^Delete / })).not.toBeInTheDocument();
		await page.getByLabelText('Name').fill('Developers');
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await page.getByRole('button', { name: 'Edit Developers' }).click();
		await expect.element(page.getByRole('heading', { name: 'Edit profile' })).toBeVisible();
		await page.getByRole('button', { name: 'Delete Developers' }).click();
		await confirmProfileDelete();

		await expect
			.element(page.getByRole('heading', { name: 'Edit profile' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Edit Developers' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit default' })).toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
		expect(savedProfiles(saved, 1).map((profile) => profile.name)).toEqual(['default']);
	});

	it('opens a profile from the footer beside delete', async () => {
		const vmcp = createVMcp('vmcp-card-footer-click');
		mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Developers');
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await page
			.getByRole('button', { name: 'Edit Developers' })
			.click({ position: { x: 24, y: 88 } });
		await expect.element(page.getByRole('heading', { name: 'Edit profile' })).toBeVisible();
		await expect.element(page.getByLabelText('Name')).toHaveValue('Developers');
	});

	it('requires a unique, non-empty name and at least one identity', async () => {
		const vmcp = createVMcp('vmcp-profile-validation');
		mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expect.element(page.getByRole('alert')).toHaveTextContent('Enter a profile name.');
		await expect.element(page.getByLabelText('Name')).toHaveAttribute('aria-invalid', 'true');

		await page.getByLabelText('Name').fill('Developers');
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('Assign at least one person or group.');
		await expect
			.element(page.getByRole('combobox', { name: 'Add identities...' }))
			.toHaveAttribute('aria-invalid', 'true');
		await expect.element(page.getByLabelText('Name')).not.toHaveAttribute('aria-invalid', 'true');

		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Developers');
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('A profile with this name already exists.');
		await expect.element(page.getByLabelText('Name')).toHaveAttribute('aria-invalid', 'true');
	});

	it('writes collected tool overrides onto the profile and the vMCP component', async () => {
		const vmcp = createVMcp('vmcp-refine-tools', false);
		const saved = mockVMcpSave(vmcp);
		const collectComponentTools = vi.fn(
			(_component, _vmcp, onCollected: Parameters<VMcpToolFlow['collectComponentTools']>[2]) => {
				onCollected({
					name: 'GitHub',
					mcpCatalogID: 'default',
					mcpServerCatalogEntryID: 'github',
					catalogEntry: { manifest: { name: 'GitHub', runtime: 'remote' } },
					toolOverrides: [
						{ name: 'list_issues', enabled: true },
						{ name: 'list_pulls', enabled: true }
					]
				});
			}
		);
		render(VMcpProfiles, {
			vmcp,
			toolFlow: toolFlowStub({ collectComponentTools })
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('button', { name: 'Refine tools' }).click();

		expect(collectComponentTools).toHaveBeenCalledOnce();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
		await expect.element(page.getByText('list_issues')).toBeVisible();
		await expect.element(page.getByText('list_pulls')).toBeVisible();
		await expect.element(toolSwitch()).toBeChecked();
		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect((saved.mock.calls[0][0] as VMCPManifest).components?.[0].toolOverrides).toEqual([
			{ name: 'list_issues', enabled: true },
			{ name: 'list_pulls', enabled: true }
		]);
	});

	it('does not offer refine when the server already has tools', async () => {
		const collectComponentTools = vi.fn();
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-refresh-tools'),
			toolFlow: toolFlowStub({ collectComponentTools })
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Refine tools' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Refresh tool overrides' }))
			.not.toBeInTheDocument();
		expect(collectComponentTools).not.toHaveBeenCalled();
	});

	it('lists profile-modifiable tools above tools locked by the vMCP', async () => {
		const vmcp = createVMcp('vmcp-locked-tools-last');
		vmcp.components![0].toolOverrides = [
			{ name: 'list_pulls', enabled: false },
			{ name: 'list_issues', enabled: true }
		];
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expandServerTools();

		await expect.element(toolSwitch(0)).toBeChecked();
		await expect.element(toolSwitch(0)).toBeEnabled();
		await expect.element(toolSwitch(1)).not.toBeChecked();
		await expect.element(toolSwitch(1)).toBeDisabled();
	});

	it('saves a locked component tool as disabled even when enabled is omitted', async () => {
		const vmcp = createVMcp('vmcp-locked-omitted-enabled');
		vmcp.components![0].toolOverrides = [
			{ name: 'list_issues', enabled: true },
			{ name: 'list_pulls' }
		];
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Support engineers');
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();
		await expandServerTools();
		await expect.element(toolSwitch(1)).not.toBeChecked();
		await expect.element(toolSwitch(1)).toBeDisabled();
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)[1]).toMatchObject({
			name: 'Support engineers',
			vmcpPermissions: { allowedComponents: { github: { allowedTools: ['list_issues'] } } }
		});
	});

	it('keeps a tool the vMCP has disabled off and out of reach', async () => {
		const vmcp = createVMcpWithDisabledTool('vmcp-disabled-tool');
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Support engineers');
		await expandServerTools();

		const disabled = toolSwitch(1);
		await expect.element(disabled).not.toBeChecked();
		await expect.element(disabled).toBeDisabled();
		await expect.element(page.getByText('Disabled on this vMCP.')).toBeVisible();
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();

		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)[1]).toMatchObject({
			name: 'Support engineers',
			vmcpPermissions: { allowedComponents: { github: { allowedTools: ['list_issues'] } } }
		});
	});

	it('collapses and expands the tool override list', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-collapse-tools'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expandServerTools();
		await expect.element(page.getByText('list_issues')).toBeVisible();

		await page.getByRole('button', { name: 'Collapse' }).click();
		await expect.element(page.getByText('list_issues')).not.toBeInTheDocument();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();

		await expandServerTools();
		await expect.element(page.getByText('list_issues')).toBeVisible();
	});

	it('toggles tools when the server row is clicked', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-row-toggle-tools'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByText('2 of 2 tools').click();
		await expect.element(page.getByText('list_issues')).toBeVisible();

		await page.getByText('GitHub').click();
		await expect.element(page.getByText('list_issues')).not.toBeInTheDocument();
	});

	it('disables a tool without removing it from the profile list', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-tool-dropdown'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expandServerTools();
		await toolSwitch(1).click();

		await expect.element(page.getByText('1 of 2 tools')).toBeVisible();
		await expect.element(page.getByText('list_pulls')).toBeVisible();
		await toolSwitch(1).click();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
	});

	it('shows Default or the enabled tool count on each server chip', async () => {
		const vmcp = createVMcp('vmcp-profile-server-chips');
		vmcp.profiles = [
			{
				name: 'default',
				subjects: [{ type: 'selector', id: '*' }],
				vmcpPermissions: { allowAllComponents: true }
			},
			{
				name: 'Limited tools',
				subjects: [{ type: 'selector', id: '*' }],
				vmcpPermissions: { allowedComponents: { github: { allowedTools: ['list_issues'] } } }
			}
		];
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await expect.element(page.getByRole('button', { name: 'Edit default' })).toBeVisible();
		await expect.element(page.getByText('Default', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Edit Limited tools' })).toBeVisible();
		await expect.element(page.getByText('1 of 2')).toBeVisible();
		await expect.element(page.getByText('GitHub')).not.toBeInTheDocument();
	});

	it('assigns people from the search dropdown', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-people-dropdown'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('combobox', { name: 'Add identities...' }).click();
		await page.getByRole('button', { name: 'All Obot Users', exact: true }).click();

		await expect.element(page.getByText('All Obot Users', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Remove All Obot Users' }).click();
		await expect.element(page.getByText('No people or groups assigned.')).toBeVisible();
	});

	it('shows a group name on the profile card instead of its directory id', async () => {
		const group = { id: 'entra/engineering-oid', name: 'Platform Engineering' };
		worker.use(
			http.get('/api/groups', ({ request }) => {
				const ids = new URL(request.url).searchParams.get('ids') ?? '';
				return HttpResponse.json({
					items: ids.split(',').includes(group.id) ? [group] : []
				});
			})
		);
		const vmcp = createVMcp('vmcp-group-card-name');
		vmcp.profiles = [
			{
				name: 'Limited tools',
				subjects: [{ type: 'group', id: group.id }],
				vmcpPermissions: { allowAllComponents: true }
			}
		];
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await expect.element(page.getByText('Platform Engineering')).toBeVisible();
		await expect.element(page.getByText(group.id)).not.toBeInTheDocument();
	});

	it('keeps a selected group name after the search listing no longer includes it', async () => {
		const group = { id: 'entra/engineering-oid', name: 'Platform Engineering' };
		worker.use(
			http.get('/api/groups', ({ request }) => {
				const url = new URL(request.url);
				if (url.searchParams.get('ids')) {
					return HttpResponse.json({ items: [group] });
				}
				if (url.searchParams.get('name')) {
					return HttpResponse.json({ items: [group] });
				}
				return HttpResponse.json({ items: [{ id: 'entra/other', name: 'Other Team' }] });
			})
		);
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-group-picker-name'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('combobox', { name: 'Add identities...' }).click();
		await page.getByPlaceholder('Search users or groups...').fill('Platform');
		await expect.element(page.getByRole('button', { name: 'Platform Engineering' })).toBeVisible();
		await page.getByRole('button', { name: 'Platform Engineering' }).click();

		await expect.element(page.getByText('Platform Engineering', { exact: true })).toBeVisible();
		await expect.element(page.getByText(group.id)).not.toBeInTheDocument();
	});

	it('keeps search and toggle all from changing tools locked by the vMCP', async () => {
		const vmcp = createVMcpWithDisabledTool('vmcp-locked-search-toggle');
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expandServerTools();

		await expect.element(page.getByPlaceholder('Search tools...')).toBeVisible();
		const toggleAll = page.getByRole('switch', { name: 'Enable All Tools' });
		await expect.element(toggleAll).toBeVisible();
		await expect.element(toggleAll).toBeChecked();

		const unlocked = toolSwitch(0);
		const locked = toolSwitch(1);
		await expect.element(unlocked).toBeChecked();
		await expect.element(unlocked).toBeEnabled();
		await expect.element(locked).not.toBeChecked();
		await expect.element(locked).toBeDisabled();

		await toggleAll.click();
		await expect.element(page.getByText('0 of 1 tools')).toBeVisible();
		await expect.element(unlocked).not.toBeChecked();
		await expect.element(locked).not.toBeChecked();
		await expect.element(locked).toBeDisabled();
		await expect.element(page.getByText('Disabled on this vMCP.')).toBeVisible();

		await toggleAll.click();
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();
		await expect.element(unlocked).toBeChecked();
		await expect.element(locked).not.toBeChecked();
		await expect.element(locked).toBeDisabled();

		await page.getByPlaceholder('Search tools...').fill('pulls');
		await expect.element(page.getByText('list_pulls')).toBeVisible();
		await expect.element(page.getByText('list_issues')).not.toBeInTheDocument();
		await expect.element(toolSwitch()).not.toBeChecked();
		await expect.element(toolSwitch()).toBeDisabled();
		await expect.element(page.getByText('Disabled on this vMCP.')).toBeVisible();
	});
});
