import { UserService } from '$lib/services';
import ClaudeConnectionProfileForm from './ClaudeConnectionProfileForm.svelte';
import type { ClaudeEnvironment, EnvironmentProfile } from './types';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

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
