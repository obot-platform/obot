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
		await expect.element(page.getByText('gmail needs OAuth authorization')).toBeVisible();
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
});
