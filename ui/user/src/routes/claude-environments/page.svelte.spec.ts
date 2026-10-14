import { page as appPage } from '$app/state';
import { profile } from '$lib/stores';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import Sessions from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => {
	cleanup();
	appPage.url.searchParams.delete('view');
});

it('shows session status and confirms workspace deletion', async () => {
	await preparePageData({ version: { engine: 'kubernetes', claudeEnvironmentsEnabled: true } });
	appPage.url.searchParams.set('view', 'sessions');
	const removed = vi.fn();
	worker.use(
		http.get('*/api/claude-sessions', () =>
			HttpResponse.json({
				items: removed.mock.calls.length
					? []
					: [
							{
								metadata: { name: 'session-one' },
								spec: {
									displayName: 'Mailbox agent',
									claudeSession: 'claude-one',
									slackSenderName: 'Donnie Adams',
									slackSenderHandle: 'thedadams',
									slackChannelName: 'craig-test',
									slackLink: 'https://acorn-io.slack.com/archives/C123/p123?thread_ts=1.23',
									claudeDone: true,
									claudeFinishedAt: '2026-01-01T12:00:00Z',
									claudeRepositories: [
										{ url: 'https://github.com/obot-platform/obot', revision: 'main' }
									]
								},
								status: { state: 'SUSPENDED' }
							}
						]
			})
		),
		http.delete('*/api/claude-sessions/session-one', () => {
			removed();
			return new HttpResponse(null, { status: 204 });
		})
	);
	await render(Sessions);
	await expect
		.element(page.getByRole('button', { name: 'Claude Environments', exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'My Claude Sessions', exact: true }))
		.toBeVisible();
	await expect.element(page.getByText('Suspended', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Donnie Adams', { exact: false })).toBeVisible();
	await expect.element(page.getByText('@thedadams', { exact: true })).toBeVisible();
	await expect.element(page.getByText('#craig-test', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('link', { name: 'Open Slack thread' }))
		.toHaveAttribute('href', 'https://acorn-io.slack.com/archives/C123/p123?thread_ts=1.23');
	await expect.element(page.getByText('Last used', { exact: true })).toBeVisible();
	expect(document.querySelector('time')?.getAttribute('datetime')).toBe('2026-01-01T12:00:00Z');
	await expect
		.element(page.getByText('https://github.com/obot-platform/obot', { exact: false }))
		.toBeVisible();
	await expect.element(page.getByText('main', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Delete workspace', exact: true }).click();
	await expect
		.element(
			page.getByText(
				'All execution attempts and saved workspace files will be permanently removed.'
			)
		)
		.toBeVisible();
	expect(removed).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Delete workspace', exact: true }).last().click();
	await vi.waitFor(() => expect(removed).toHaveBeenCalledTimes(1));
	await expect
		.element(
			page.getByText('No sessions yet. Start a session in Claude using your connected environment.')
		)
		.toBeVisible();
});

it('shows all sessions with owner attribution', async () => {
	await preparePageData({ version: { engine: 'kubernetes', claudeEnvironmentsEnabled: true } });
	appPage.url.searchParams.set('view', 'all-sessions');
	const requested = vi.fn();
	worker.use(
		http.get('*/api/users', () =>
			HttpResponse.json({
				items: [{ id: '42', displayName: 'Slack service', email: 'slack@example.com' }]
			})
		),
		http.get('*/api/claude-sessions', ({ request }) => {
			requested(new URL(request.url).searchParams.get('all'));
			return HttpResponse.json({
				items: [
					{
						metadata: { name: 'tag-session' },
						spec: {
							displayName: 'Shared agent',
							claudeSession: 'claude-tag-session',
							userID: '42',
							claudeEnvironment: 'env-one',
							claudeAccount: 'claude-tag',
							claudeDone: true,
							claudeFinishedAt: '2026-01-01T12:00:00Z',
							claudeRepositories: [
								{ url: 'https://github.com/obot-platform/obot', revision: 'main' },
								{ url: 'https://github.com/agent-substrate/substrate', revision: 'v0.3.0' }
							]
						},
						status: { state: 'SUSPENDED', error: 'Runner unavailable' }
					}
				]
			});
		})
	);
	await render(Sessions);
	await expect
		.element(page.getByRole('button', { name: 'All Claude Sessions', exact: true }))
		.toBeVisible();
	await expect.element(page.getByText('Suspended', { exact: true })).toBeVisible();
	await expect
		.element(page.getByText('Claude Tag · Slack service slack@example.com', { exact: true }))
		.toBeVisible();
	await expect.element(page.getByText('slack@example.com', { exact: true })).toBeVisible();
	await expect
		.element(page.getByText('https://github.com/obot-platform/obot', { exact: false }))
		.toBeVisible();
	await expect
		.element(page.getByText('https://github.com/agent-substrate/substrate', { exact: false }))
		.toBeVisible();
	await expect.element(page.getByText('v0.3.0', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Last used', { exact: true })).toBeVisible();
	expect(document.querySelector('time')?.getAttribute('datetime')).toBe('2026-01-01T12:00:00Z');
	await expect.element(page.getByText('env-one', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Runner unavailable', { exact: true })).toBeVisible();
	expect(requested).toHaveBeenCalledWith('true');
});

it('does not expose the all-sessions view to ordinary users', async () => {
	await preparePageData({ version: { engine: 'kubernetes', claudeEnvironmentsEnabled: true } });
	profile.initialize(createMockProfile([]));
	appPage.url.searchParams.set('view', 'all-sessions');
	const requested = vi.fn();
	worker.use(
		http.get('*/api/claude-environments', () => HttpResponse.json({ enabled: false })),
		http.get('*/api/claude-sessions', () => {
			requested();
			return HttpResponse.json({ items: [] });
		})
	);
	await render(Sessions);
	await expect.element(page.getByText('Claude environments are disabled.')).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'All Claude Sessions', exact: true }))
		.not.toBeInTheDocument();
	expect(requested).not.toHaveBeenCalled();
});

it('lets admins request suspension and shows the pending state', async () => {
	await preparePageData({ version: { engine: 'kubernetes', claudeEnvironmentsEnabled: true } });
	appPage.url.searchParams.set('view', 'all-sessions');
	let suspended = false;
	worker.use(
		http.get('*/api/claude-sessions', () =>
			HttpResponse.json({
				items: [
					{
						metadata: { name: 'running-session' },
						spec: {
							displayName: 'Running agent',
							claudeSession: 'session',
							userID: '3',
							claudeEnvironment: 'env',
							suspended
						},
						status: { state: 'RUNNING' }
					}
				]
			})
		),
		http.post('*/api/claude-sessions/running-session/suspend', () => {
			suspended = true;
			return new HttpResponse(null, { status: 202 });
		})
	);
	await render(Sessions);
	await page.getByRole('button', { name: 'Suspend', exact: true }).click();
	expect(suspended).toBe(false);
	await page.getByRole('button', { name: 'Suspend session', exact: true }).click();
	await expect.element(page.getByText('Suspending', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Suspend', exact: true }))
		.not.toBeInTheDocument();
});

it('explains Kubernetes is required on Docker without showing sessions', async () => {
	await preparePageData({ version: { engine: 'docker', claudeEnvironmentsEnabled: false } });
	await render(Sessions);
	await expect
		.element(
			page.getByText(
				'Claude Environments is only available in Kubernetes deployments. This Obot instance is running with Docker.'
			)
		)
		.toBeVisible();
	expect(page.getByRole('button', { name: 'My Claude Sessions', exact: true }).all()).toHaveLength(
		0
	);
	expect(page.getByRole('button', { name: 'Add environment', exact: true }).all()).toHaveLength(0);
});

it.each(['sessions', 'all-sessions'])(
	'hides disabled session views opened directly: %s',
	async (view) => {
		await preparePageData({ version: { engine: 'kubernetes', claudeEnvironmentsEnabled: false } });
		appPage.url.searchParams.set('view', view);
		const requested = vi.fn();
		worker.use(
			http.get('*/api/claude-environments', () => HttpResponse.json({ enabled: false })),
			http.get('*/api/claude-sessions', () => {
				requested();
				return HttpResponse.json({ items: [] });
			})
		);
		await render(Sessions);
		await expect.element(page.getByText('Claude environments are disabled.')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'My Claude Sessions', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'All Claude Sessions', exact: true }))
			.not.toBeInTheDocument();
		expect(requested).not.toHaveBeenCalled();
	}
);
