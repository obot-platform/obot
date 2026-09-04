import type { VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import type { VMCP, VMCPManifest } from '$lib/services';
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

async function expandServerTools() {
	await page.getByRole('button', { name: 'Expand' }).click();
}

async function assignEveryone() {
	await page.getByRole('combobox', { name: 'Search identities...' }).click();
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
		await page.getByRole('checkbox').nth(1).click();
		await expect.element(page.getByText('1 of 2 tools')).toBeVisible();
		await page.getByRole('button', { name: 'Customize' }).first().click();
		await page.getByLabelText('Tool name').fill('github_list_issues');
		await expect.element(page.getByText('github_list_issues')).toBeVisible();
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await expect
			.element(page.getByRole('button', { name: 'Edit Support engineers' }))
			.toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)).toEqual([
			{ name: 'default', subjects: [{ type: 'selector', id: '*' }], allowAllTools: true },
			{
				name: 'Support engineers',
				subjects: [{ type: 'selector', id: '*' }],
				allowAllTools: false,
				allowedTools: { github: ['list_issues'] }
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

		await page.getByRole('button', { name: 'Edit Developers' }).click();
		await page.getByLabelText('Name').fill('Platform developers');
		await page.getByRole('button', { name: 'Save changes' }).click();

		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Delete Platform developers' }).click();
		await page.getByRole('button', { name: 'Cancel' }).click();
		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.toBeVisible();

		await page.getByRole('button', { name: 'Delete Platform developers' }).click();
		await confirmProfileDelete();

		await expect
			.element(page.getByRole('button', { name: 'Edit Platform developers' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit default' })).toBeVisible();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(3));
		expect(savedProfiles(saved, 1).map((profile) => profile.name)).toEqual([
			'default',
			'Platform developers'
		]);
		expect(savedProfiles(saved, 2).map((profile) => profile.name)).toEqual(['default']);
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

		await expect.element(page.getByRole('heading', { name: 'Edit profile' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit Developers' })).not.toBeInTheDocument();
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

		await page.getByRole('button', { name: 'Edit Developers' }).click({ position: { x: 24, y: 88 } });
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
			.element(page.getByRole('combobox', { name: 'Search identities...' }))
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
		await expect.element(page.getByRole('checkbox').first()).toBeChecked();
		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect((saved.mock.calls[0][0] as VMCPManifest).components?.[0].toolOverrides).toEqual([
			{ name: 'list_issues', enabled: true },
			{ name: 'list_pulls', enabled: true }
		]);
	});

	it('refreshes existing tool overrides without updating the vMCP', async () => {
		const collectComponentTools = vi.fn(
			(_component, _vmcp, onCollected: Parameters<VMcpToolFlow['collectComponentTools']>[2]) => {
				onCollected({
					name: 'GitHub',
					mcpCatalogID: 'default',
					mcpServerCatalogEntryID: 'github',
					catalogEntry: { manifest: { name: 'GitHub', runtime: 'remote' } },
					toolOverrides: [
						{ name: 'list_issues', enabled: true },
						{ name: 'search_code', enabled: true }
					]
				});
			}
		);
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-refresh-tools'),
			toolFlow: toolFlowStub({ collectComponentTools })
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('button', { name: 'Refresh tool overrides' }).click();

		expect(collectComponentTools).toHaveBeenCalledOnce();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
		await expect.element(page.getByText('search_code')).toBeVisible();
		await expect.element(page.getByText('list_pulls')).not.toBeInTheDocument();
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

		await expect.element(page.getByRole('checkbox').nth(0)).toBeChecked();
		await expect.element(page.getByRole('checkbox').nth(0)).toBeEnabled();
		await expect.element(page.getByRole('checkbox').nth(1)).not.toBeChecked();
		await expect.element(page.getByRole('checkbox').nth(1)).toBeDisabled();
	});

	it('saves a locked component tool as disabled even when enabled is omitted', async () => {
		const vmcp = createVMcp('vmcp-locked-omitted-enabled');
		vmcp.components![0].toolOverrides = [{ name: 'list_issues', enabled: true }, { name: 'list_pulls' }];
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Support engineers');
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();
		await expandServerTools();
		await expect.element(page.getByRole('checkbox').nth(1)).not.toBeChecked();
		await expect.element(page.getByRole('checkbox').nth(1)).toBeDisabled();
		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)[1]).toMatchObject({
			name: 'Support engineers',
			allowedTools: { github: ['list_issues'] }
		});
	});

	it('keeps a tool the vMCP has disabled off and out of reach', async () => {
		const vmcp = createVMcpWithDisabledTool('vmcp-disabled-tool');
		const saved = mockVMcpSave(vmcp);
		render(VMcpProfiles, { vmcp, toolFlow: toolFlowStub() });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByLabelText('Name').fill('Support engineers');
		await expandServerTools();

		const disabled = page.getByRole('checkbox').nth(1);
		await expect.element(disabled).not.toBeChecked();
		await expect.element(disabled).toBeDisabled();
		await expect.element(page.getByText('Disabled on this vMCP.')).toBeVisible();
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();

		await assignEveryone();
		await page.getByRole('button', { name: 'Create profile', exact: true }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(savedProfiles(saved)[1]).toMatchObject({
			name: 'Support engineers',
			allowedTools: { github: ['list_issues'] }
		});
	});

	it('refuses tools the vMCP has disabled when overrides are refreshed', async () => {
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
			vmcp: createVMcpWithDisabledTool('vmcp-disabled-tool-refresh'),
			toolFlow: toolFlowStub({ collectComponentTools })
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('button', { name: 'Refresh tool overrides' }).click();

		expect(collectComponentTools).toHaveBeenCalledOnce();
		await expect.element(page.getByText('1 of 1 tools')).toBeVisible();
		await expect.element(page.getByRole('checkbox').nth(1)).not.toBeChecked();
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

	it('disables a tool without removing it from the profile list', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-tool-dropdown'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await expandServerTools();
		await page.getByRole('checkbox').nth(1).click();

		await expect.element(page.getByText('1 of 2 tools')).toBeVisible();
		await expect.element(page.getByText('list_pulls')).toBeVisible();
		await page.getByRole('checkbox').nth(1).click();
		await expect.element(page.getByText('2 of 2 tools')).toBeVisible();
	});

	it('assigns people from the search dropdown', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-people-dropdown'),
			toolFlow: toolFlowStub()
		});

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await page.getByRole('combobox', { name: 'Search identities...' }).click();
		await page.getByRole('button', { name: 'All Obot Users', exact: true }).click();

		await expect.element(page.getByText('All Obot Users', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Remove All Obot Users' }).click();
		await expect.element(page.getByText('No people or groups assigned.')).toBeVisible();
	});

	it('keeps the people picker on its placeholder after assigning and removing', async () => {
		render(VMcpProfiles, {
			vmcp: createVMcp('vmcp-people-placeholder'),
			toolFlow: toolFlowStub()
		});

		const picker = page.getByRole('combobox', { name: 'Search identities...' });

		await page.getByRole('button', { name: 'Create profile', exact: true }).click();
		await picker.click();
		await page.getByRole('button', { name: 'All Obot Users', exact: true }).click();
		await expect.element(picker).toHaveTextContent('Search identities...');

		await page.getByRole('button', { name: 'Remove All Obot Users' }).click();
		await expect.element(page.getByText('No people or groups assigned.')).toBeVisible();
		await expect.element(picker).toHaveTextContent('Search identities...');
	});
});
