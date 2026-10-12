import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import SearchUsers from './SearchUsers.svelte';
import { http, HttpResponse } from 'msw';
import { untrack } from 'svelte';
import { describe, expect, it } from 'vitest';
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
});
