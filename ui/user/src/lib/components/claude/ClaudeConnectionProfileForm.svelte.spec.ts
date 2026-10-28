import {
	UserService,
	type ClaudeEnvironment,
	type VMCP,
	type EnvironmentProfile
} from '$lib/services';
import ClaudeConnectionProfileForm from './ClaudeConnectionProfileForm.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const environment = {
	id: 'one',
	displayName: 'Development',
	environmentID: 'ccpool_one',
	organizationID: 'org',
	claudeTagEnabled: false,
	profiles: [],
	runtime: {
		image: 'example/runner:latest',
		workerImage: 'example/worker:latest',
		egressHosts: [],
		worker: { workerReplicas: 1, cpuLimit: '1', memoryLimit: '1Gi' },
		resources: {}
	}
} satisfies ClaudeEnvironment;

const servers = [
	{ id: 'vmcp-one', displayName: 'Outlook' },
	{ id: 'vmcp-two', displayName: 'GitHub' },
	{ id: 'vmcp-three', displayName: 'Slack' }
] as VMCP[];

function profile(name: string, mcpServerIDs: string[] | null): EnvironmentProfile {
	return { name, subjects: [], mcpServerIDs, resources: {} };
}

const profiles = [
	profile('default', null),
	profile('bare', []),
	profile('on-call', ['vmcp-one', 'vmcp-two']),
	profile('writers', ['vmcp-three'])
];

function renderForm(overrides: { profile: EnvironmentProfile; editingName?: string }) {
	return render(ClaudeConnectionProfileForm, {
		environment,
		profiles,
		servers,
		onClose: () => {},
		onSaved: () => {},
		...overrides
	});
}

it('adds vMCPs from a profile that has them and skips profiles that do not', async () => {
	renderForm({ profile: profile('developers', ['vmcp-one']) });

	await page
		.getByRole('button', { name: 'Import vMCPs from existing profile', exact: true })
		.click();
	const dialog = page.getByRole('dialog', { name: 'Select Profile To Import' });
	await expect.element(dialog.getByText('on-call', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('Outlook', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('GitHub', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('writers', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('Slack', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('default', { exact: true })).not.toBeInTheDocument();
	await expect.element(dialog.getByText('bare', { exact: true })).not.toBeInTheDocument();
	await expect
		.element(dialog.getByRole('button', { name: 'Import vMCPs', exact: true }))
		.toBeDisabled();

	await dialog.getByRole('radio', { name: /on-call/ }).click();
	await dialog.getByRole('button', { name: 'Import vMCPs', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'Remove GitHub', exact: true }))
		.toBeVisible();
	expect(page.getByRole('button', { name: 'Remove Outlook', exact: true }).all()).toHaveLength(1);
	expect(page.getByRole('button', { name: 'Remove GitHub', exact: true }).all()).toHaveLength(1);
});

it('omits the profile being edited from the import list', async () => {
	renderForm({ profile: profile('on-call', ['vmcp-one', 'vmcp-two']), editingName: 'on-call' });

	await page
		.getByRole('button', { name: 'Import vMCPs from existing profile', exact: true })
		.click();
	const dialog = page.getByRole('dialog', { name: 'Select Profile To Import' });
	await expect.element(dialog.getByText('writers', { exact: true })).toBeVisible();
	await expect.element(dialog.getByText('on-call', { exact: true })).not.toBeInTheDocument();
	await expect.element(dialog.getByText('Outlook', { exact: true })).not.toBeInTheDocument();
	await expect.element(dialog.getByText('default', { exact: true })).not.toBeInTheDocument();
});

it.each([undefined, true])('edits Claude MCP access from %s', async (allowClaudeMCP) => {
	vi.spyOn(UserService, 'listUsers').mockResolvedValue([]);
	vi.spyOn(UserService, 'listGroups').mockResolvedValue({
		items: [],
		source: 'cache',
		degraded: false,
		reset: false
	});
	const profile: EnvironmentProfile = {
		name: 'default',
		subjects: [],
		mcpServerIDs: [],
		resources: {},
		allowClaudeMCP
	};
	const environment: ClaudeEnvironment = {
		id: '',
		displayName: 'Development',
		organizationID: '',
		environmentID: '',
		claudeTagEnabled: false,
		profiles: [profile],
		runtime: {
			image: '',
			workerImage: '',
			egressHosts: [],
			worker: { workerReplicas: 1, cpuLimit: '2', memoryLimit: '4Gi' },
			resources: {}
		}
	};
	const saved = vi.fn();
	await render(ClaudeConnectionProfileForm, {
		environment,
		profiles: [profile],
		profile,
		editingName: profile.name,
		onClose: () => {},
		onSaved: saved
	});
	const toggle = page.getByRole('checkbox', { name: 'Allow Claude MCP', exact: true });
	if (allowClaudeMCP) {
		await expect.element(toggle).toBeChecked();
	} else {
		await expect.element(toggle).not.toBeChecked();
	}
	await toggle.click();
	await page.getByRole('button', { name: 'Apply', exact: true }).click();
	await expect.poll(() => saved.mock.calls[0]?.[0][0].allowClaudeMCP).toBe(!allowClaudeMCP);
});
