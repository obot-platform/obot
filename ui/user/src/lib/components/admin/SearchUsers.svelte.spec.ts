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

		it('offers no agents or kind wildcards unless asked to', async () => {
			await openPicker({});

			await expect.element(page.getByText('Build bot')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: /All Agents/ }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Users/ })).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: /and Agents/ }))
				.not.toBeInTheDocument();
		});

		it('offers active agents and the wildcards when asked to', async () => {
			await openPicker({ offerAgents: true });

			await expect.element(page.getByText('Build bot')).toBeVisible();
			await expect.element(page.getByText('Paused bot')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: /Everyone \(All Obot Users and Agents\)/ }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: /All Users/ })).toBeVisible();
			await expect.element(page.getByRole('button', { name: /All Agents/ })).toBeVisible();
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

		it('disables the other wildcards once Everyone is selected', async () => {
			const onAdd = await openPicker({ offerAgents: true });

			await page.getByRole('button', { name: /Everyone \(All Obot Users and Agents\)/ }).click();

			await expect.element(page.getByRole('button', { name: /All Users/ })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: /All Agents/ })).toBeDisabled();
			// Individual users and agents are not wildcards, so they stay available.
			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();

			await page.getByRole('button', { name: 'Confirm' }).click();
			expect(onAdd).toHaveBeenCalledWith([], [{ id: '*', name: expect.any(String) }], []);
		});

		it('disables Everyone once all users or all agents are selected', async () => {
			await openPicker({ offerAgents: true });

			await page.getByRole('button', { name: /All Users/ }).click();

			await expect
				.element(page.getByRole('button', { name: /Everyone \(All Obot Users and Agents\)/ }))
				.toBeDisabled();
			// All users with all agents is everyone, which has its own wildcard.
			await expect.element(page.getByRole('button', { name: /All Agents/ })).toBeDisabled();
		});

		it('disables the wildcards that conflict with a subject that is already added', async () => {
			await openPicker({ offerAgents: true, filterIds: ['*'] });

			await expect.element(page.getByRole('button', { name: /All Users/ })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: /All Agents/ })).toBeDisabled();
		});

		it('makes a wildcard exclusive with other subjects when it must be alone', async () => {
			await openPicker({ offerAgents: true, wildcardMustBeAlone: true, filterIds: ['agent:9'] });

			await expect.element(page.getByRole('button', { name: /All Agents/ })).toBeDisabled();
			await expect
				.element(page.getByRole('button', { name: /Everyone \(All Obot Users and Agents\)/ }))
				.toBeDisabled();
			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeEnabled();
		});

		it('disables every other subject once a wildcard is added and it must be alone', async () => {
			await openPicker({ offerAgents: true, wildcardMustBeAlone: true, filterIds: ['agents:*'] });

			await expect.element(page.getByRole('button', { name: /Build bot/ })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: /Alice/ })).toBeDisabled();
		});
	});
});
