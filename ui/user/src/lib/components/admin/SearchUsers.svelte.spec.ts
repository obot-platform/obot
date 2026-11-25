import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import SearchUsers from './SearchUsers.svelte';
import { http, HttpResponse } from 'msw';
import { untrack } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('SearchUsers', () => {
	it('does not offer disabled users', async () => {
		worker.use(
			http.get('*/api/users', () =>
				HttpResponse.json({
					items: [
						{ id: 'u1', displayName: 'Active Alice', status: 'active' },
						{ id: 'u2', displayName: 'Disabled Dan', status: 'disabled' }
					]
				})
			)
		);
		await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
		const { component } = await render(SearchUsers, { onAdd: () => {} });

		untrack(() => component.open());

		await expect.element(page.getByText('Active Alice')).toBeVisible();
		await expect.element(page.getByText('Disabled Dan')).not.toBeInTheDocument();
	});

	describe('agents', () => {
		const agents = [
			{ id: '5', displayName: 'Build bot', status: 'active', activeAPIKeyCount: 1 },
			{ id: '6', displayName: 'Paused bot', status: 'disabled', activeAPIKeyCount: 0 }
		];

		async function openPicker(props: Record<string, unknown>) {
			worker.use(
				http.get('*/api/agents', () => HttpResponse.json({ items: agents })),
				http.get('*/api/users', () =>
					HttpResponse.json({ items: [{ id: '5', displayName: 'Alice', status: 'active' }] })
				)
			);
			await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
			const onAdd = vi.fn();
			const { component } = await render(SearchUsers, { onAdd, ...props });
			untrack(() => component.open());
			await expect.element(page.getByText('Alice')).toBeVisible();
			return onAdd;
		}

		it('offers no agents unless asked to, and the wildcard as All Obot Users', async () => {
			await openPicker({});

			await expect.element(page.getByText('Build bot')).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: /All Agents/ }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Users/ })).not.toBeInTheDocument();
		});

		it('offers active agents by name, and still only All Obot Users as the wildcard', async () => {
			await openPicker({ offerAgents: true });

			await expect.element(page.getByText('Build bot')).toBeVisible();
			await expect.element(page.getByText('Paused bot')).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: /All Agents/ }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Users/ })).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: /and Agents/ }))
				.not.toBeInTheDocument();
		});

		it('keeps a user and an agent with the same ID apart', async () => {
			const onAdd = await openPicker({ offerAgents: true });

			await page.getByRole('button', { name: /Build bot/ }).click();
			await page.getByRole('button', { name: /Alice/ }).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			expect(onAdd).toHaveBeenCalledWith(
				[expect.objectContaining({ id: '5' })],
				[],
				[expect.objectContaining({ id: '5', displayName: 'Build bot' })]
			);
		});

		it('lets All Obot Users be picked next to agents when it need not be alone', async () => {
			const onAdd = await openPicker({ offerAgents: true });

			await page.getByRole('button', { name: /All Obot Users/ }).click();
			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();
			await page.getByRole('button', { name: /Build bot/ }).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			expect(onAdd).toHaveBeenCalledWith(
				[],
				[{ id: '*', name: 'All Obot Users' }],
				[expect.objectContaining({ id: '5' })]
			);
		});

		it('disables All Obot Users next to a user when it must be alone', async () => {
			// A user who is already a subject: not listed here, but All Obot Users can't join them.
			await openPicker({ offerAgents: true, wildcardMustBeAlone: true, filterIds: ['7'] });

			await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();
		});

		// All Obot Users doesn't include agents, so a policy for every user names its agents next to it.
		it('lets All Obot Users be added next to an agent when it must be alone', async () => {
			await openPicker({ offerAgents: true, wildcardMustBeAlone: true, filterIds: ['agent:9'] });

			await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeEnabled();
			await expect.element(page.getByRole('button', { name: /Alice/ })).toBeEnabled();
		});

		it('lets agents, but no user, be added once All Obot Users is added and it must be alone', async () => {
			const onAdd = await openPicker({
				offerAgents: true,
				wildcardMustBeAlone: true,
				filterIds: ['*']
			});

			await expect.element(page.getByRole('button', { name: /Alice/ })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();
			await page.getByRole('button', { name: /Build bot/ }).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			expect(onAdd).toHaveBeenCalledWith([], [], [expect.objectContaining({ id: '5' })]);
		});
	});
});
