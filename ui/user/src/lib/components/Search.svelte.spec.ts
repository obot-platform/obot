import SearchRerenderHarness from '../../tests/fixtures/SearchRerenderHarness.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Search', () => {
	it('preserves typed text when the parent rerenders before the debounced change', async () => {
		const onChange = vi.fn();
		render(SearchRerenderHarness, { onChange });
		const input = page.getByRole('textbox');

		await input.fill('server');
		await page.getByRole('button', { name: 'Rerender' }).click();

		await expect.element(input).toHaveValue('server');
	});

	it('ignores a stale echo after the user continues typing', async () => {
		const onChange = vi.fn();
		render(SearchRerenderHarness, { onChange });
		const input = page.getByRole('textbox');

		await input.fill('server');
		await vi.waitFor(() => expect(onChange).toHaveBeenCalledWith('server'));
		await input.fill('servers');
		await page.getByRole('button', { name: 'Echo change' }).click();

		await expect.element(input).toHaveValue('servers');
	});
});
