import { Group, type SkillAccessPolicy } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import SkillAccessPolicyForm from './SkillAccessPolicyForm.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

// A rule that is not saved yet (it has no ID) and already has its name and resources, so that only the
// subjects remain to be chosen.
const draft = {
	displayName: 'Skills for bots',
	subjects: [],
	resources: [{ type: 'selector', id: '*' }]
} as unknown as SkillAccessPolicy;

function mockApis() {
	const created = vi.fn();
	worker.use(
		http.get('/api/skills', () => HttpResponse.json({ items: [] })),
		http.get('/api/skill-repositories', () => HttpResponse.json({ items: [] })),
		http.get('/api/agents', () =>
			HttpResponse.json({
				items: [
					{ id: '5', displayName: 'Build bot', status: 'active', activeAPIKeyCount: 1 },
					{ id: '9', displayName: 'Retired bot', status: 'deleted', activeAPIKeyCount: 0 }
				]
			})
		),
		http.post('/api/skill-access-rules', async ({ request }) => {
			const body = (await request.json()) as Record<string, unknown>;
			created(body);
			return HttpResponse.json({ ...body, id: 'sar1', created: '2026-01-01T00:00:00Z' });
		})
	);
	return created;
}

async function openPicker() {
	await page.getByRole('button', { name: 'Add User/Group' }).click();
	await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeVisible();
}

beforeEach(async () => {
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

describe('SkillAccessPolicyForm: agent subjects', () => {
	it('saves a selected agent as an agent subject', async () => {
		const created = mockApis();
		render(SkillAccessPolicyForm, { skillAccessPolicy: { ...draft } });

		await openPicker();
		await page.getByRole('button', { name: /Build bot/ }).click();
		await page.getByRole('button', { name: 'Confirm' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalled());
		expect(created.mock.calls[0][0]).toMatchObject({ subjects: [{ type: 'agent', id: '5' }] });
	});

	it('saves All Obot Users as the selector wildcard, with an agent next to it', async () => {
		const created = mockApis();
		render(SkillAccessPolicyForm, { skillAccessPolicy: { ...draft } });

		await openPicker();
		await page.getByRole('button', { name: /All Obot Users/ }).click();
		// All Obot Users doesn't include agents, so a rule for every user names its agents next to it.
		await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();
		await page.getByRole('button', { name: /Build bot/ }).click();
		await page.getByRole('button', { name: 'Confirm' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalled());
		expect(created.mock.calls[0][0].subjects).toEqual(
			expect.arrayContaining([
				{ type: 'selector', id: '*' },
				expect.objectContaining({ type: 'agent' })
			])
		);
	});

	it('shows a deleted agent as such', async () => {
		mockApis();
		render(SkillAccessPolicyForm, {
			skillAccessPolicy: { ...draft, subjects: [{ type: 'agent', id: '9' }] }
		});

		await expect.element(page.getByText('Retired bot (Deleted)')).toBeVisible();
	});
});
