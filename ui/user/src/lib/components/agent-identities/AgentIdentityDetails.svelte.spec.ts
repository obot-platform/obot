import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import {
	createAgent,
	createAgentKey,
	emptyAgentAccess,
	readyVMCP
} from '../../../tests/mocks/agentIdentities';
import { worker } from '../../../tests/mocks/worker';
import AgentIdentityDetails from './AgentIdentityDetails.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const revoked = vi.fn();
const auditQueries: string[] = [];

beforeEach(async () => {
	revoked.mockReset();
	auditQueries.length = 0;
	worker.use(
		http.get('/api/agents/7/access', () =>
			HttpResponse.json({
				...emptyAgentAccess,
				problems: [{ code: 'no_model_access_policy' }]
			})
		),
		http.get('/api/agents/7/vmcps', () => HttpResponse.json({ items: [readyVMCP] })),
		http.get('/api/admin-audit-events', ({ request }) => {
			const search = new URL(request.url).search;
			auditQueries.push(search);
			if (!search.includes('resourceKind=agent')) {
				return HttpResponse.json({ items: [], total: 0, limit: 100, offset: 0 });
			}
			return HttpResponse.json({
				items: [
					{
						id: 1,
						createdAt: '2026-01-01T00:00:00Z',
						actor: { actorType: 'user', id: '1', displayName: 'Ada Admin' },
						action: 'agent.create',
						resourceKind: 'agent',
						resourceID: '7'
					}
				],
				total: 1,
				limit: 100,
				offset: 0
			});
		}),
		http.delete('/api/agents/7/api-keys/11', () => {
			revoked();
			return new HttpResponse(null, { status: 204 });
		})
	);
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

describe('AgentIdentityDetails', () => {
	it('shows the masked key, access problems, readiness and history', async () => {
		render(AgentIdentityDetails, {
			agent: createAgent(),
			keys: [createAgentKey()],
			creatorName: 'Ada Admin',
			onDeleted: vi.fn()
		});

		await expect.element(page.getByText('oak1-7-11-*****')).toBeVisible();
		// Keys from the UI can use everything the agent can, so the table lists no capabilities.
		await expect
			.element(page.getByRole('columnheader', { name: 'Capabilities' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText(/no LLM gateway policy names this agent/)).toBeVisible();
		await expect.element(page.getByText('LLM gateway policies')).toBeVisible();
		await expect.element(page.getByText('Mail vMCP')).toBeVisible();
		await expect.element(page.getByText(/^gmail uses the user's own account/)).toBeVisible();
		await expect.element(page.getByText('Agent created')).toBeVisible();
		// Key events are asked for by key ID, since the audit has no filter by parent.
		expect(auditQueries.some((q) => q.includes('resourceKind=apiKey') && q.includes('11'))).toBe(
			true
		);
	});

	it('revokes a key after a confirmation and hides it from the list', async () => {
		render(AgentIdentityDetails, {
			agent: createAgent(),
			keys: [createAgentKey()],
			onDeleted: vi.fn()
		});

		await page.getByRole('button', { name: 'Revoke', exact: true }).click();
		await expect.element(page.getByText('Revoke the key "ci key"?')).toBeVisible();
		await page.getByRole('button', { name: 'Revoke', exact: true }).last().click();

		await expect.poll(() => revoked.mock.calls.length).toBe(1);
		await expect.element(page.getByText('oak1-7-11-*****')).not.toBeInTheDocument();
	});

	it.each([
		{ name: 'auditors', groups: [Group.USER, Group.AUDITOR] },
		{ name: 'read-only admins', groups: [Group.USER, Group.ADMIN_READONLY] }
	])('hides mutations for $name', async ({ groups }) => {
		await preparePageData({ profile: createMockProfile(groups) });
		render(AgentIdentityDetails, {
			agent: createAgent(),
			keys: [createAgentKey()],
			readonly: true,
			onDeleted: vi.fn()
		});

		await expect.element(page.getByText('oak1-7-11-*****')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Create API Key' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete', exact: true }))
			.not.toBeInTheDocument();
	});

	describe('whole-object saves', () => {
		const profiles = [
			{ id: 'ap1all', name: 'Everyone', subjects: [{ type: 'selector', id: '*' }] }
		];
		const putBodies: Record<string, unknown>[] = [];

		beforeEach(() => {
			putBodies.length = 0;
			worker.use(
				http.get('*/api/users', () => HttpResponse.json({ items: [] })),
				http.put('/api/agents/7', async ({ request }) => {
					const body = (await request.json()) as Record<string, unknown>;
					putBodies.push(body);
					// The server leaves out a list that is empty.
					const { profiles: sent, allowedReturnHosts: hosts, ...rest } = body;
					return HttpResponse.json({
						...createAgent(),
						...rest,
						...((sent as unknown[]).length > 0 ? { profiles: sent } : {}),
						...((hosts as unknown[]).length > 0 ? { allowedReturnHosts: hosts } : {})
					});
				})
			);
		});

		it('sends the agent profiles and the return hosts back when the name is changed', async () => {
			render(AgentIdentityDetails, {
				agent: createAgent({
					profiles: profiles as never,
					allowedReturnHosts: ['a.example.com']
				}),
				keys: [],
				onDeleted: vi.fn()
			});

			await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Renamed Bot');
			await page.getByRole('button', { name: 'Save', exact: true }).click();

			await vi.waitFor(() => expect(putBodies).toHaveLength(1));
			expect(putBodies[0]).toMatchObject({
				displayName: 'Renamed Bot',
				profiles,
				allowedReturnHosts: ['a.example.com']
			});
		});

		it('shows the agent profiles and the delegations on their own tabs', async () => {
			worker.use(
				http.get('/api/agents/7/delegations', () =>
					HttpResponse.json({
						items: [
							{
								id: 'adl1ada',
								agent: { id: '7', displayName: 'Build Bot', status: 'active' },
								user: { id: 'u1', displayName: 'Ada Admin' },
								firstApprovedAt: '2026-01-01T00:00:00Z',
								lastApprovedAt: '2026-01-01T00:00:00Z'
							}
						]
					})
				)
			);
			render(AgentIdentityDetails, {
				agent: createAgent({ profiles: profiles as never }),
				keys: [createAgentKey()],
				onDeleted: vi.fn()
			});

			await expect.element(page.getByText('oak1-7-11-*****')).toBeVisible();
			await page.getByRole('tab', { name: 'User access' }).click();
			await expect.element(page.getByRole('row', { name: /Everyone/ })).toBeVisible();
			await expect.element(page.getByText('oak1-7-11-*****')).not.toBeInTheDocument();

			await page.getByRole('tab', { name: 'Delegations' }).click();
			await expect.element(page.getByText('Ada Admin')).toBeVisible();
		});

		it('saves the saved name with the agent profiles, not what is typed in the form', async () => {
			render(AgentIdentityDetails, {
				agent: createAgent({
					profiles: profiles as never,
					allowedReturnHosts: ['a.example.com']
				}),
				keys: [],
				onDeleted: vi.fn()
			});

			await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Unsaved name');
			await page.getByRole('tab', { name: 'User access' }).click();
			await page
				.getByRole('row', { name: /Everyone/ })
				.getByRole('button', { name: 'Delete' })
				.click();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(putBodies).toHaveLength(1));
			expect(putBodies[0]).toEqual({
				displayName: 'Build Bot',
				description: 'Runs builds',
				profiles: [],
				allowedReturnHosts: ['a.example.com']
			});
			// The server left the empty list out, and the list is empty.
			await expect.element(page.getByText(/No user can approve this agent/)).toBeVisible();
		});

		it('saves new return hosts with the agent profiles', async () => {
			render(AgentIdentityDetails, {
				agent: createAgent({ profiles: profiles as never }),
				keys: [],
				onDeleted: vi.fn()
			});

			await page.getByRole('tab', { name: 'User access' }).click();
			await page.getByLabelText('Host').fill('agent.example.com');
			await page.getByRole('button', { name: 'Save Hosts' }).click();

			await vi.waitFor(() => expect(putBodies).toHaveLength(1));
			expect(putBodies[0]).toMatchObject({ profiles, allowedReturnHosts: ['agent.example.com'] });
		});

		it.each([
			{ name: 'auditors', groups: [Group.USER, Group.AUDITOR] },
			{ name: 'read-only admins', groups: [Group.USER, Group.ADMIN_READONLY] }
		])('keeps the tabs read-only for $name', async ({ groups }) => {
			await preparePageData({ profile: createMockProfile(groups) });
			worker.use(http.get('/api/agents/7/delegations', () => HttpResponse.json({ items: [] })));
			render(AgentIdentityDetails, {
				agent: createAgent({ profiles: profiles as never }),
				keys: [],
				readonly: true,
				onDeleted: vi.fn()
			});

			await page.getByRole('tab', { name: 'User access' }).click();
			await expect.element(page.getByRole('row', { name: /Everyone/ })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Add User Access' }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Save Hosts' }))
				.not.toBeInTheDocument();
		});
	});
});
