import { UserService, type VMCP } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ClaudeConnection from './ClaudeConnection.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

beforeEach(() => {
	worker.use(http.get('*/api/claude-sessions', () => HttpResponse.json({ items: [] })));
});

const runtime = {
	image: 'example/runner@sha256:' + 'a'.repeat(64),
	workerImage: 'example/worker:v0.3.0',
	egressHosts: [],
	worker: {
		workerReplicas: 1,
		cpuLimit: '2',
		memoryLimit: '4Gi'
	},
	resources: { actorCPU: '1', actorMemory: '2Gi', idleTimeoutMinutes: 5, sessionRetentionDays: 30 }
};

it('shows a mapped environment with session statistics and without account editing', async () => {
	profile.initialize(createMockProfile([]));
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([
		{ id: 'vmcp', displayName: 'Outlook' } as VMCP
	]);
	const sessionsRequest = vi.fn();
	worker.use(
		http.get('*/api/claude-sessions', ({ request }) => {
			sessionsRequest(new URL(request.url).search);
			return HttpResponse.json({
				items: [
					{ metadata: {}, spec: { claudeEnvironment: 'one' }, status: { state: 'RUNNING' } },
					{
						metadata: {},
						spec: {
							claudeEnvironment: 'one',
							claudeDone: true,
							claudeFinishedAt: '2026-10-01T12:00:00Z'
						},
						status: { state: 'RUNNING' }
					},
					{ metadata: {}, spec: { claudeEnvironment: 'one' }, status: { state: 'PENDING' } },
					{ metadata: {}, spec: { claudeEnvironment: 'other' }, status: { state: 'RUNNING' } }
				]
			});
		})
	);

	worker.use(
		http.get('*/api/claude-environments', () =>
			HttpResponse.json({
				enabled: true,
				environments: [
					{
						id: 'one',
						displayName: 'Development',
						environmentID: 'ccpool_one',
						organizationID: 'org',
						mcpServerIDs: ['vmcp'],
						profiles: [],
						runtime: { ...runtime, egressHosts: ['*.github.com'], allowAllEgress: false },
						egressSecrets: [{ hostname: 'private.example.com', header: 'X-Secret' }]
					}
				],
				mappings: [{ organizationID: 'org', account: 'user_account', email: 'user@example.com' }]
			})
		)
	);
	render(ClaudeConnection);
	await expect.element(page.getByText('Development', { exact: true })).toBeVisible();
	await expect
		.element(
			page.getByText('claude "Describe your task" --environment ccpool_one', { exact: true })
		)
		.toBeVisible();
	await expect.element(page.getByText('user_account', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Active now', { exact: true })).toBeVisible();
	await expect.element(page.getByText('3', { exact: true })).toBeVisible();
	await expect.element(page.getByText('1', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Outlook', { exact: true })).toBeVisible();
	await expect.element(page.getByText('*.github.com', { exact: true })).toBeVisible();
	expect(page.getByText('Worker pool', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('Injected secrets', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('private.example.com', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('Actor:', { exact: false }).all()).toHaveLength(0);
	expect(sessionsRequest).toHaveBeenCalledWith('');
	expect(page.getByRole('combobox').all()).toHaveLength(0);
	expect(page.getByRole('button', { name: 'Add environment' }).all()).toHaveLength(0);
});

it('saves worker settings on the environment and actor overrides on profiles', async () => {
	profile.initialize(createMockProfile());
	vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([
		{ id: 'vmcp-two', displayName: 'GitHub' },
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
	render(ClaudeConnection);
	await page.getByRole('button', { name: 'Add environment', exact: true }).click();
	await page.getByLabelText('Display name', { exact: true }).fill('Development');
	expect(page.getByLabelText('Claude organization ID').all()).toHaveLength(0);
	expect(page.getByLabelText('Claude environment ID').all()).toHaveLength(0);
	await page.getByLabelText('Environment key', { exact: true }).fill('test-key');
	await page.getByRole('combobox', { name: 'Select vMCPs…' }).click();
	await page.getByPlaceholder('Search vMCPs…').fill('Out');
	await page.getByRole('button', { name: 'Outlook', exact: true }).click();
	await page.getByRole('combobox', { name: 'Select vMCPs…' }).click();
	await page.getByPlaceholder('Search vMCPs…').fill('');
	await expect.element(page.getByRole('button', { name: 'Outlook', exact: true })).toBeVisible();
	expect(document.querySelector('#environment-vmcps-popover button')?.textContent?.trim()).toBe(
		'Outlook'
	);
	await page.getByRole('button', { name: 'GitHub', exact: true }).click();

	await page.getByRole('combobox', { name: 'Select vMCPs…' }).click();
	await page.getByLabelText('Allow all environment vMCPs').click();
	await page.getByRole('combobox', { name: 'Select vMCPs…' }).nth(1).click();
	await page.getByPlaceholder('Search vMCPs…').nth(1).fill('Out');
	await page.getByRole('button', { name: 'Outlook', exact: true }).click();
	await page.getByRole('combobox', { name: 'Select vMCPs…' }).nth(1).click();
	await page.getByPlaceholder('Search vMCPs…').nth(1).fill('');
	expect(document.querySelector('#profile-vmcps-0-popover button')?.textContent?.trim()).toBe(
		'Outlook'
	);

	await page.getByRole('combobox', { name: 'Select vMCPs…' }).nth(1).click();

	expect(page.getByText('Advanced configuration', { exact: true }).all()).toHaveLength(2);
	await expect.element(page.getByLabelText('Worker image', { exact: true })).not.toBeVisible();
	await expect.element(page.getByLabelText('Runner image', { exact: true })).not.toBeVisible();
	expect(page.getByLabelText('Snapshot location').all()).toHaveLength(0);
	expect(page.getByLabelText('Worker CPU request').all()).toHaveLength(0);
	expect(page.getByLabelText('Worker memory request').all()).toHaveLength(0);
	await page.getByText('Advanced configuration', { exact: true }).nth(1).click();
	await expect
		.element(page.getByLabelText('Runner image', { exact: true }))
		.toHaveValue(runtime.image);
	await expect
		.element(page.getByLabelText('Allow all egress', { exact: true }).first())
		.not.toBeChecked();
	await page.getByRole('button', { name: 'Add hostname', exact: true }).first().click();
	await page.getByLabelText('Additional HTTPS egress hosts').fill(' *.github.com ');
	await page.getByRole('button', { name: 'Add hostname', exact: true }).first().click();
	await page.getByLabelText('Additional HTTPS egress hosts').nth(1).fill('remove.example.com');
	await page.getByRole('button', { name: 'Remove hostname', exact: true }).nth(1).click();
	await page.getByLabelText('Allow all egress', { exact: true }).first().click();
	await expect.element(page.getByLabelText('Additional HTTPS egress hosts')).toBeDisabled();
	await page.getByLabelText('Runner image', { exact: true }).fill('example/runner:main');
	await page.getByRole('button', { name: 'Save environment' }).click();
	expect(saved).not.toHaveBeenCalled();
	await page.getByLabelText('Managed settings JSON', { exact: true }).fill('[]');
	await page.getByLabelText('Runner image', { exact: true }).fill(runtime.image);
	await page.getByRole('button', { name: 'Save environment' }).click();
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
	await page.getByRole('button', { name: 'Add hostname', exact: true }).nth(1).click();
	await page.getByLabelText('Additional HTTPS hosts for this profile').fill('*.example.com');
	await page.getByRole('button', { name: 'Add hostname', exact: true }).nth(1).click();
	await page.getByLabelText('Allow all egress', { exact: true }).nth(1).click();
	await page.getByRole('button', { name: 'Add secret', exact: true }).first().click();
	await page.getByLabelText('Destination hostname', { exact: true }).fill('api.example.com');
	await page.getByLabelText('Header name', { exact: true }).fill('Authorization');
	await page.getByLabelText('Secret value', { exact: true }).fill('Bearer env-secret');
	await page.getByRole('button', { name: 'Add secret', exact: true }).nth(1).click();
	await page.getByLabelText('Destination hostname', { exact: true }).nth(1).fill('api.example.com');
	await page.getByLabelText('Header name', { exact: true }).nth(1).fill('Authorization');
	await page.getByLabelText('Secret value', { exact: true }).nth(1).fill('Bearer profile-secret');
	await page.getByText('Actor overrides', { exact: true }).click();
	await page.getByLabelText('Actor CPU limit', { exact: true }).nth(1).fill('1500m');
	await page.getByRole('button', { name: 'Save environment' }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				mcpServerIDs: ['vmcp-two', 'vmcp-one'],
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
						mcpServerIDs: ['vmcp-one'],
						egressSecrets: [
							expect.objectContaining({
								hostname: 'api.example.com',
								header: 'Authorization',
								value: 'Bearer profile-secret'
							})
						],
						egressHosts: ['*.example.com'],
						allowAllEgress: true,
						resources: expect.objectContaining({ actorCPU: '1500m' })
					})
				]
			})
		)
	);
	expect(saved.mock.calls[0][0]).not.toHaveProperty('organizationID');
	expect(saved.mock.calls[0][0]).not.toHaveProperty('environmentID');
	expect(saved.mock.calls[0][0].runtime).not.toHaveProperty('snapshotLocation');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('cpuRequest');
	expect(saved.mock.calls[0][0].runtime.worker).not.toHaveProperty('memoryRequest');
});

it.each([false, true])(
	'shows empty vMCPs and egress policy with allowAllEgress=%s',
	async (allowAllEgress) => {
		profile.initialize(createMockProfile([]));
		vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
		worker.use(
			http.get('*/api/claude-environments', () =>
				HttpResponse.json({
					enabled: true,
					environments: [
						{
							id: 'one',
							displayName: 'Development',
							environmentID: 'pool',
							organizationID: 'org',
							mcpServerIDs: [],
							profiles: [],
							runtime: { ...runtime, allowAllEgress }
						}
					]
				})
			)
		);
		render(ClaudeConnection);
		await expect.element(page.getByText('No vMCPs available.', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText(
					allowAllEgress ? 'All HTTP and HTTPS egress is allowed.' : 'No additional egress hosts.',
					{ exact: true }
				)
			)
			.toBeVisible();
		await expect
			.element(page.getByText('Traffic to internal Obot services', { exact: false }))
			.toBeVisible();
	}
);
