import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import SelectServerType from './SelectServerType.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('offers hosted and remote servers, but not legacy composites', async () => {
	profile.initialize(createMockProfile());
	const onSelectServerType = vi.fn();
	const result = await render(SelectServerType, { onSelectServerType });
	result.component.open();

	await expect.element(page.getByRole('button', { name: /Hosted Server/ })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: /Composite Server/ }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: /Remote Server/ }).click();
	expect(onSelectServerType).toHaveBeenCalledWith('remote');
});
