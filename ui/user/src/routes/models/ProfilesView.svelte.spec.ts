import { Group } from '$lib/services';
import { worker } from '../../tests/mocks/worker';
import { renderModelsTab, resetModelsTab } from './testing';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

const profiles = [
	{
		id: 'p-everyone',
		displayName: 'Everyone',
		subjects: [{ type: 'selector', id: '*' }],
		models: [{ id: 'obot://llm' }]
	},
	{
		id: 'p-eng',
		displayName: 'Engineering',
		subjects: [
			{ type: 'group', id: 'okta/eng' },
			{ type: 'user', id: '1' },
			{ type: 'user', id: '2' }
		],
		models: [{ id: '*' }]
	}
];

function mockProfiles() {
	const deleted = vi.fn();
	worker.use(
		http.get('/api/model-access-policies', () => HttpResponse.json({ items: profiles })),
		http.get('/api/llm-gateway/budgets', () =>
			HttpResponse.json({
				items: [
					{ id: 'b1', name: 'Daily', profileID: 'p-everyone', limits: [] },
					{ id: 'b2', name: 'Eng spend', profileID: 'p-eng', limits: [] },
					{ id: 'b3', name: 'Eng tokens', profileID: 'p-eng', limits: [] }
				]
			})
		),
		http.get('/api/llm-gateway/guardrail-policies', () =>
			HttpResponse.json({
				items: [{ id: 'r1', name: 'Secrets', target: 'llm', profileID: 'p-everyone' }]
			})
		),
		http.delete('/api/model-access-policies/:id', ({ params }) => {
			deleted(params.id);
			return new HttpResponse(null, { status: 204 });
		})
	);
	return { deleted };
}

afterEach(resetModelsTab);

describe('Models profiles tab', () => {
	it('shows each profile as a card with its users, models, budgets and applied guardrails', async () => {
		mockProfiles();
		await renderModelsTab('profiles');

		const eng = page.getByRole('article').filter({ hasText: 'Engineering' });
		await expect.element(eng.getByText('2 users, 1 group')).toBeInTheDocument();
		await expect.element(eng.getByText('All models')).toBeInTheDocument();
		await expect.element(eng.getByText('2 budgets')).toBeInTheDocument();
		await expect.element(eng.getByText('0 applied guardrails')).toBeInTheDocument();

		const everyone = page.getByRole('article').filter({ hasText: 'Everyone' }).first();
		await expect.element(everyone.getByText('1 model', { exact: true })).toBeInTheDocument();
		await expect.element(everyone.getByText('1 budget', { exact: true })).toBeInTheDocument();
		await expect
			.element(everyone.getByText('1 applied guardrail', { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'Edit Engineering' }))
			.toHaveAttribute('href', '/models/profiles/p-eng');
	});

	it('deletes a profile after saying its budgets and applied guardrails go with it', async () => {
		const { deleted } = mockProfiles();
		await renderModelsTab('profiles');

		await expect.element(page.getByText('2 users, 1 group')).toBeInTheDocument();
		const card = page.getByText('2 users, 1 group').element().closest('article') as HTMLElement;
		(card.querySelector('button[aria-label="Delete profile"]') as HTMLElement).click();
		await expect
			.element(
				page.getByText('Its budgets and applied guardrails are deleted with it', { exact: false })
			)
			.toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('p-eng'));
	});

	it('hides deleting from auditors', async () => {
		mockProfiles();
		await renderModelsTab('profiles', [Group.USER, Group.AUDITOR]);

		await expect.element(page.getByText('2 users, 1 group')).toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete profile' }))
			.not.toBeInTheDocument();
	});
});
