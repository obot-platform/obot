import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import IncludeOnBehalfOfToggle from './IncludeOnBehalfOfToggle.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const label = 'Include actions on behalf of this user';

afterEach(() => {
	(appPage.url as URL).searchParams.delete('include_on_behalf_of');
	vi.mocked(goto).mockReset();
});

describe('IncludeOnBehalfOfToggle', () => {
	it('sets include_on_behalf_of when it is checked', async () => {
		render(IncludeOnBehalfOfToggle);

		await page.getByRole('checkbox', { name: label }).click();

		expect(goto).toHaveBeenCalledTimes(1);
		const url = vi.mocked(goto).mock.calls[0][0] as URL;
		expect(url.searchParams.get('include_on_behalf_of')).toBe('true');
	});

	it('is checked from the URL, and removes the parameter when it is unchecked', async () => {
		(appPage.url as URL).searchParams.set('include_on_behalf_of', 'true');
		render(IncludeOnBehalfOfToggle);

		const checkbox = page.getByRole('checkbox', { name: label });
		await expect.element(checkbox).toBeChecked();
		await checkbox.click();

		const url = vi.mocked(goto).mock.calls[0][0] as URL;
		expect(url.searchParams.has('include_on_behalf_of')).toBe(false);
	});
});
