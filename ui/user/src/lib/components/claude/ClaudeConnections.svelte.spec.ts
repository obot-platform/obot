import { UserService } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ClaudeConnections from './ClaudeConnections.svelte';
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
	const listVMCPs = vi.spyOn(UserService, 'listVMCPs').mockResolvedValue([]);
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
						profiles: null,
						runtime: { ...runtime, egressHosts: ['*.github.com'], allowAllEgress: false },
						egressSecrets: [{ hostname: 'private.example.com', header: 'X-Secret' }]
					}
				],
				mappings: [{ organizationID: 'org', account: 'user_account', email: 'user@example.com' }]
			})
		)
	);
	render(ClaudeConnections, { onEdit: () => {} });
	await expect
		.element(page.getByRole('heading', { level: 2, name: 'Development', exact: false }))
		.toBeVisible();
	await expect
		.element(
			page.getByText('claude "Describe your task" --environment ccpool_one', { exact: true })
		)
		.toBeVisible();
	await expect.element(page.getByText('user_account', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Active now', { exact: true })).toBeVisible();
	await expect.element(page.getByText('3', { exact: true })).toBeVisible();
	await expect.element(page.getByText('1', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('heading', { name: 'Available vMCPs', exact: true }))
		.not.toBeInTheDocument();
	expect(listVMCPs).not.toHaveBeenCalled();
	await expect.element(page.getByText('*.github.com', { exact: true })).toBeVisible();
	expect(page.getByText('Worker Pool', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('Secrets', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('private.example.com', { exact: true }).all()).toHaveLength(0);
	expect(page.getByText('Actor:', { exact: false }).all()).toHaveLength(0);
	expect(sessionsRequest).toHaveBeenCalledWith('');
	expect(page.getByRole('combobox').all()).toHaveLength(0);
	expect(page.getByRole('button', { name: 'Edit Development' }).all()).toHaveLength(0);
});

it('opens an environment for editing when an admin selects its card', async () => {
	profile.initialize(createMockProfile());
	const onEdit = vi.fn();
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
						profiles: [],
						runtime
					}
				]
			})
		)
	);
	render(ClaudeConnections, { onEdit });
	const edit = page.getByRole('button', { name: 'Edit Development', exact: true });
	await expect.element(edit).toBeVisible();
	const el = await edit.element();
	if (!(el instanceof HTMLElement))
		throw new Error('Expected the edit control to be an HTMLElement');
	el.click();
	expect(onEdit).toHaveBeenCalledWith(
		expect.objectContaining({ id: 'one', displayName: 'Development' })
	);
	expect(page.getByText('No vMCPs available.', { exact: true }).all()).toHaveLength(0);
});

it.each([false, true])(
	'shows egress policy without a vMCP section with allowAllEgress=%s',
	async (allowAllEgress) => {
		profile.initialize(createMockProfile([]));
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
							profiles: null,
							runtime: { ...runtime, allowAllEgress }
						}
					]
				})
			)
		);
		render(ClaudeConnections, { onEdit: () => {} });
		await expect
			.element(
				page.getByText(
					allowAllEgress
						? 'HTTP on port 80 and HTTPS on port 443 are allowed to any hostname.'
						: 'No additional egress hosts.',
					{ exact: true }
				)
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Available vMCPs', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText('No vMCPs available.', { exact: true }))
			.not.toBeInTheDocument();
	}
);
