import { Group, UserService, type ClaudeEnvironment, type VMCP } from '$lib/services';
import { profile } from '$lib/stores';
import { success } from '$lib/stores/success';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ClaudeConnectionForm from './ClaudeConnectionForm.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const runtime = {
	image: 'example/runner@sha256:' + 'a'.repeat(64),
	workerImage: 'example/worker:v0.3.0',
	egressHosts: [] as string[],
	worker: {
		workerReplicas: 1,
		cpuLimit: '2',
		memoryLimit: '4Gi'
	},
	resources: { actorCPU: '1', actorMemory: '2Gi', idleTimeoutMinutes: 5, sessionRetentionDays: 30 }
};

const environment = {
	id: 'one',
	displayName: 'Development',
	environmentID: 'ccpool_one',
	organizationID: 'org',
	mcpServerIDs: ['vmcp-one'],
	profiles: [] as unknown[],
	runtime,
	claudeTagEnabled: false
};

it('saves a new environment from the details form with a default profile', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([
		{ id: 'vmcp-two', displayName: 'GitHub' },
		{ id: 'vmcp-one', displayName: 'Outlook' }
	] as VMCP[]);
	const saved = vi.fn();
	const submitted = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.post('*/api/claude-environments', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: submitted });
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	expect(page.getByLabelText('Claude organization ID').all()).toHaveLength(0);
	expect(page.getByLabelText('Claude environment ID').all()).toHaveLength(0);
	expect(page.getByRole('button', { name: 'Profiles', exact: true }).all()).toHaveLength(1);
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');

	expect(page.getByText('Advanced configuration', { exact: true }).all()).toHaveLength(1);
	await expect.element(page.getByLabelText('Worker image', { exact: true })).toBeVisible();
	await expect
		.element(page.getByText('Worker tolerations', { exact: true }))
		.not.toBeInTheDocument();
	await expect.element(page.getByLabelText('Worker node selectors')).not.toBeInTheDocument();
	await expect.element(page.getByLabelText('Runner image', { exact: true })).not.toBeVisible();
	expect(page.getByLabelText('Snapshot location').all()).toHaveLength(0);
	expect(page.getByLabelText('Worker CPU request').all()).toHaveLength(0);
	expect(page.getByLabelText('Worker memory request').all()).toHaveLength(0);
	await page.getByText('Advanced configuration', { exact: true }).click();
	// A blank image follows Obot's default, which the field shows as its placeholder.
	await expect.element(page.getByLabelText('Runner image', { exact: true })).toHaveValue('');
	await expect
		.element(page.getByLabelText('Runner image', { exact: true }))
		.toHaveAttribute('placeholder', runtime.image);
	await expect
		.element(page.getByLabelText('Allow all web destinations', { exact: true }))
		.not.toBeChecked();
	await expect
		.element(
			page.getByText('Allow HTTP on port 80 and HTTPS on port 443 to any hostname.', {
				exact: false
			})
		)
		.toBeVisible();
	await page.getByRole('button', { name: 'Add hostname', exact: true }).click();
	await page.getByLabelText('Additional HTTPS egress hosts').fill(' *.github.com ');
	await page.getByRole('button', { name: 'Add hostname', exact: true }).click();
	await page.getByLabelText('Additional HTTPS egress hosts').nth(1).fill('remove.example.com');
	await page.getByRole('button', { name: 'Remove hostname', exact: true }).nth(1).click();
	await page.getByLabelText('Allow all web destinations', { exact: true }).click();
	await expect
		.element(page.getByLabelText('Additional HTTPS egress hosts'))
		.not.toBeInTheDocument();
	await page.getByLabelText('Runner image', { exact: true }).fill('example/runner:main');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	expect(saved).not.toHaveBeenCalled();
	await page.getByLabelText('Managed settings JSON', { exact: true }).fill('[]');
	await page.getByLabelText('Runner image', { exact: true }).fill(runtime.image);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect
		.element(page.getByText('Managed settings must be a JSON object.', { exact: true }))
		.toBeVisible();
	expect(saved).not.toHaveBeenCalled();
	await page
		.getByLabelText('Managed settings JSON', { exact: true })
		.fill('{"permissions":{"deny":["Read(./secret)"]}}');
	await page.getByLabelText('Worker replicas', { exact: true }).fill('2');
	await page.getByLabelText('Worker CPU limit', { exact: true }).fill('1');
	await page.getByLabelText('Worker memory limit', { exact: true }).fill('2Gi');
	await page.getByRole('button', { name: 'Add secret', exact: true }).click();
	await page.getByLabelText('Destination hostname', { exact: true }).fill('api.example.com');
	await page.getByLabelText('Header name', { exact: true }).fill('Authorization');
	await page.getByLabelText('Secret value', { exact: true }).fill('Bearer env-secret');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				displayName: 'Development',
				environmentKey: 'test-key',
				egressSecrets: [
					expect.objectContaining({
						hostname: 'api.example.com',
						header: 'Authorization',
						value: 'Bearer env-secret'
					})
				],
				runtime: expect.objectContaining({
					allowAllEgress: true,
					egressHosts: ['*.github.com'],
					managedSettings: { permissions: { deny: ['Read(./secret)'] } },
					worker: expect.objectContaining({ workerReplicas: 2, cpuLimit: '1', memoryLimit: '2Gi' })
				}),
				profiles: [
					expect.objectContaining({
						name: 'default',
						mcpServerIDs: null,
						subjects: [{ type: 'obotGroup', id: Group.ADMIN }]
					})
				]
			})
		)
	);
	expect(saved.mock.calls[0][0]).not.toHaveProperty('mcpServerIDs');
	expect(saved.mock.calls[0][0]).not.toHaveProperty('organizationID');
	expect(saved.mock.calls[0][0]).not.toHaveProperty('environmentID');
	expect(saved.mock.calls[0][0].runtime).not.toHaveProperty('snapshotLocation');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('cpuRequest');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('memoryRequest');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('nodeSelector');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('tolerations');
	await vi.waitFor(() => expect(submitted).toHaveBeenCalled());
}, 5000);

it('saves blank images so environments follow the default images', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.post('*/api/claude-environments', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');
	await expect.element(page.getByLabelText('Worker image', { exact: true })).toHaveValue('');
	await expect
		.element(page.getByLabelText('Worker image', { exact: true }))
		.toHaveAttribute('placeholder', runtime.workerImage);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].runtime).toEqual(
		expect.objectContaining({ image: '', workerImage: '', llmProxy: false })
	);
}, 5000);

it('confirms the LLM gateway requirements before turning it on', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.post('*/api/claude-environments', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');
	const toggle = page.getByLabelText("Send model requests through Obot's LLM gateway", {
		exact: true
	});
	await expect.element(toggle).not.toBeChecked();

	// Cancelling the confirmation leaves the LLM gateway off.
	await toggle.click();
	await expect
		.element(
			page.getByText('The Anthropic model provider must allow Claude subscriptions.', {
				exact: true
			})
		)
		.toBeVisible();
	await expect
		.element(
			page.getByText(
				'A model gateway profile must give all users access to Claude Opus 5.5, Sonnet 5.5, Haiku 5.5, and Fable 5.1.',
				{ exact: true }
			)
		)
		.toBeVisible();
	await page.getByRole('button', { name: 'Cancel', exact: true }).last().click();
	await expect.element(toggle).not.toBeChecked();

	await toggle.click();
	await page.getByRole('button', { name: 'Use the LLM gateway', exact: true }).click();
	await expect.element(toggle).toBeChecked();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].runtime).toEqual(expect.objectContaining({ llmProxy: true }));
}, 5000);

it('confirms the LLM gateway again after dismissing it with Escape', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		)
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	const toggle = page.getByLabelText("Send model requests through Obot's LLM gateway", {
		exact: true
	});
	const confirm = page.getByRole('button', { name: 'Use the LLM gateway', exact: true });

	await toggle.click();
	await expect.element(confirm).toBeVisible();
	await userEvent.keyboard('{Escape}');
	await expect.element(confirm).not.toBeInTheDocument();
	await expect.element(toggle).not.toBeChecked();

	await toggle.click();
	await confirm.click();
	await expect.element(toggle).toBeChecked();
}, 5000);

it('turns the LLM gateway off without confirming', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [{ ...environment, runtime: { ...runtime, llmProxy: true } }],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.put('*/api/claude-environments/one', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'one', onCancel: () => {}, onSubmit: () => {} });
	const toggle = page.getByLabelText("Send model requests through Obot's LLM gateway", {
		exact: true
	});
	await expect.element(toggle).toBeChecked();
	await toggle.click();
	await expect.element(toggle).not.toBeChecked();
	await expect
		.element(page.getByText('The Anthropic model provider must allow Claude subscriptions.'))
		.not.toBeVisible();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].runtime).toEqual(expect.objectContaining({ llmProxy: false }));
}, 5000);

it('clears a pinned image so the environment follows the default again', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [environment],
				defaultWorkerImage: 'example/worker:v0.4.0',
				defaultRunnerImage: 'example/runner@sha256:' + 'b'.repeat(64)
			})
		),
		http.put('*/api/claude-environments/one', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'one', onCancel: () => {}, onSubmit: () => {} });
	await expect
		.element(page.getByLabelText('Worker image', { exact: true }))
		.toHaveValue(runtime.workerImage);
	await page.getByLabelText('Worker image', { exact: true }).fill('');
	await page.getByText('Advanced configuration', { exact: true }).click();
	await expect
		.element(page.getByLabelText('Runner image', { exact: true }))
		.toHaveValue(runtime.image);
	await page.getByLabelText('Runner image', { exact: true }).fill('');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].runtime).toEqual(
		expect.objectContaining({ image: '', workerImage: '' })
	);
}, 5000);

it('keeps a new profile on an unsaved environment until the connection form is saved', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	vi.spyOn(UserService, 'listUsers').mockResolvedValue([]);
	vi.spyOn(UserService, 'listGroups').mockResolvedValue({
		items: [],
		source: 'cache',
		degraded: false,
		reset: false
	});
	const saved = vi.fn();
	const updated = vi.fn();
	const notified = vi.spyOn(success, 'add');
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.post('*/api/claude-environments', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		}),
		http.put('*/api/claude-environments/*', async ({ request }) => {
			updated(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'Edit default', exact: true }))
		.toBeVisible();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await page.getByLabelText('Profile name', { exact: true }).fill('developers');
	await expect
		.element(page.getByRole('button', { name: 'Save', exact: true }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Apply', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'Edit developers', exact: true }))
		.toBeVisible();
	expect(saved).not.toHaveBeenCalled();
	expect(updated).not.toHaveBeenCalled();
	expect(notified).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Details', exact: true }).click();
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				displayName: 'Development',
				environmentKey: 'test-key',
				profiles: [
					expect.objectContaining({ name: 'default' }),
					expect.objectContaining({ name: 'developers', mcpServerIDs: null, subjects: [] })
				]
			})
		)
	);
	expect(updated).not.toHaveBeenCalled();
});

it('saves one profile onto an existing environment', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([
		{ id: 'vmcp-one', displayName: 'Outlook' }
	] as VMCP[]);
	vi.spyOn(UserService, 'listUsers').mockResolvedValue([]);
	vi.spyOn(UserService, 'listGroups').mockResolvedValue({
		items: [],
		source: 'cache',
		degraded: false,
		reset: false
	});
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({ enabled: true, environments: [environment] })
		),
		http.put('*/api/claude-environments/one', async ({ request }) => {
			const payload = (await request.json()) as ClaudeEnvironment;
			saved(payload);
			return HttpResponse.json({
				...payload,
				profiles: payload.profiles.map((item) => ({
					...item,
					egressSecrets: item.egressSecrets?.map((secret) => ({
						hostname: secret.hostname,
						header: secret.header,
						id: 'server-secret-id'
					}))
				}))
			});
		})
	);
	render(ClaudeConnectionForm, { environment: 'one', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await page.getByLabelText('Profile name', { exact: true }).fill('developers');
	await page.getByRole('combobox', { name: 'Select vMCPs…' }).click();
	await page.getByPlaceholder('Search vMCPs…').fill('Out');
	await page.getByRole('button', { name: 'Outlook', exact: true }).click();
	await expect.element(page.getByText('Outlook', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Add hostname', exact: true }).click();
	await page.getByLabelText('Additional HTTPS hosts for this profile').fill(' *.example.com ');
	await page.getByRole('button', { name: 'Add secret', exact: true }).click();
	await page.getByLabelText('Destination hostname', { exact: true }).fill('api.example.com');
	await page.getByLabelText('Header name', { exact: true }).fill('Authorization');
	await page.getByLabelText('Secret value', { exact: true }).fill('Bearer profile-secret');
	await page.getByText('Actor overrides', { exact: true }).click();
	await page.getByLabelText('Actor CPU limit', { exact: true }).fill('1500m');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				profiles: [
					expect.objectContaining({
						name: 'developers',
						mcpServerIDs: ['vmcp-one'],
						egressHosts: ['*.example.com'],
						egressSecrets: [
							expect.objectContaining({
								hostname: 'api.example.com',
								header: 'Authorization',
								value: 'Bearer profile-secret'
							})
						],
						resources: expect.objectContaining({ actorCPU: '1500m' })
					})
				]
			})
		)
	);
	expect(saved.mock.calls[0][0]).not.toHaveProperty('organizationID');
	expect(saved.mock.calls[0][0]).not.toHaveProperty('environmentID');
	await page.getByRole('button', { name: 'Edit developers', exact: true }).click();
	await page.getByLabelText('Destination hostname', { exact: true }).fill('new.example.com');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
	expect(saved.mock.calls[1][0].profiles[0].egressSecrets).toEqual([
		{ id: 'server-secret-id', hostname: 'new.example.com', header: 'Authorization' }
	]);
});

function mockProfileDirectory() {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	vi.spyOn(UserService, 'listUsers').mockResolvedValue([]);
	vi.spyOn(UserService, 'listGroups').mockResolvedValue({
		items: [],
		source: 'cache',
		degraded: false,
		reset: false
	});
}

it('leaves an unchanged profile form without an unsaved-changes prompt', async () => {
	mockProfileDirectory();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		)
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await expect.element(page.getByLabelText('Profile name', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Details', exact: true }).click();
	await expect.element(page.getByLabelText('Display name', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('dialog').filter({ hasText: 'This profile has unsaved changes.' }))
		.not.toBeInTheDocument();
});

it('applies unsaved profile changes when leaving for another tab', async () => {
	mockProfileDirectory();
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		),
		http.post('*/api/claude-environments', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await page.getByLabelText('Profile name', { exact: true }).fill('developers');
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await expect
		.element(page.getByRole('dialog').filter({ hasText: 'This profile has unsaved changes.' }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Details', exact: true }).click();
	const dialog = page.getByRole('dialog').filter({ hasText: 'This profile has unsaved changes.' });
	await expect
		.element(dialog.getByText('Would you like to apply them?', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByLabelText('Display name', { exact: true }))
		.not.toBeInTheDocument();
	await dialog.getByRole('button', { name: 'Keep editing', exact: true }).click();
	await expect.element(dialog).not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('Profile name', { exact: true }))
		.toHaveValue('developers');
	await page.getByRole('button', { name: 'Details', exact: true }).click();
	await dialog.getByRole('button', { name: 'Apply', exact: true }).click();
	await expect.element(page.getByLabelText('Display name', { exact: true })).toBeVisible();
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				profiles: expect.arrayContaining([
					expect.objectContaining({ name: 'developers', mcpServerIDs: null, subjects: [] })
				])
			})
		)
	);
});

it('keeps the profile form open when unsaved changes cannot be applied', async () => {
	mockProfileDirectory();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [],
				defaultWorkerImage: runtime.workerImage,
				defaultRunnerImage: runtime.image
			})
		)
	);
	render(ClaudeConnectionForm, { environment: 'new', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await page.getByRole('button', { name: 'Add hostname', exact: true }).click();
	await page.getByLabelText('Additional HTTPS hosts for this profile').fill('*.example.com');
	await page.getByRole('button', { name: 'Details', exact: true }).click();
	const dialog = page.getByRole('dialog').filter({ hasText: 'This profile has unsaved changes.' });
	await dialog.getByRole('button', { name: 'Apply', exact: true }).click();
	await expect.element(dialog).not.toBeInTheDocument();
	await expect.element(page.getByLabelText('Profile name', { exact: true })).toBeVisible();
	await expect
		.element(page.getByLabelText('Display name', { exact: true }))
		.not.toBeInTheDocument();
});

it('saves unsaved profile changes when switching to Claude Tag', async () => {
	mockProfileDirectory();
	const saved = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({ enabled: true, environments: [environment] })
		),
		http.put('*/api/claude-environments/one', async ({ request }) => {
			const payload = (await request.json()) as ClaudeEnvironment;
			saved(payload);
			return HttpResponse.json(payload);
		})
	);
	render(ClaudeConnectionForm, { environment: 'one', onCancel: () => {}, onSubmit: () => {} });
	await page.getByRole('button', { name: 'Profiles', exact: true }).click();
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await page.getByLabelText('Profile name', { exact: true }).fill('developers');
	await page.getByRole('button', { name: 'Claude Tag', exact: true }).click();
	const dialog = page.getByRole('dialog').filter({ hasText: 'This profile has unsaved changes.' });
	await expect
		.element(dialog.getByText('Would you like to save them?', { exact: true }))
		.toBeVisible();
	await dialog.getByRole('button', { name: 'Keep editing', exact: true }).click();
	expect(saved).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Claude Tag', exact: true }).click();
	await dialog.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				profiles: [expect.objectContaining({ name: 'developers' })]
			})
		)
	);
	await expect
		.element(page.getByLabelText('Claude Tag for Development', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByLabelText('Profile name', { exact: true }))
		.not.toBeInTheDocument();
});

it('asks for confirmation before deleting an environment', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
	const deleted = vi.fn();
	const submitted = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({ enabled: true, environments: [environment] })
		),
		http.delete('*/api/claude-environments/one', () => {
			deleted();
			return new HttpResponse(null, { status: 204 });
		})
	);
	render(ClaudeConnectionForm, { environment: 'one', onCancel: () => {}, onSubmit: submitted });
	await page.getByRole('button', { name: 'Delete', exact: true }).click();
	expect(deleted).not.toHaveBeenCalled();
	const dialog = page.getByRole('dialog').filter({
		hasText: 'Delete Development? Its session workspaces and worker pool will be removed.'
	});
	await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
	await expect.element(dialog).not.toBeInTheDocument();
	expect(deleted).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Delete', exact: true }).click();
	await dialog.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();
	await vi.waitFor(() => expect(deleted).toHaveBeenCalled());
	await vi.waitFor(() => expect(submitted).toHaveBeenCalled());
});
