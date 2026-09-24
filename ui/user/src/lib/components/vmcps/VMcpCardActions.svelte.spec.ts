import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpCardActions from './VMcpCardActions.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('keeps Connect available without offering a direct URL shortcut', async () => {
	await preparePageData();
	await render(VMcpCardActions, { id: 'test', hideTest: true });
	await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
	await expect.element(page.getByRole('button')).toHaveLength(1);
});
