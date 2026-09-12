import { goto } from '$lib/url';
import FiltersDrawer from './FiltersDrawer.svelte';
import { expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn().mockResolvedValue(undefined)
}));

test('selects, applies, restores and removes server names containing commas', async () => {
	const name = 'Outlook, Calendar';
	const props = {
		filters: { mcp_server: '' },
		onClose: vi.fn(),
		getUserDisplayName: (id: string) => id,
		endpoint: async () => ({ options: [name, 'Other server'] })
	};
	const view = await render(FiltersDrawer, props);
	const select = page.getByRole('combobox');
	await select.click({ position: { x: 5, y: 5 } });
	await page.getByRole('button', { name, exact: true }).click();
	await expect.element(select).toHaveTextContent(name);
	await expect.element(page.getByRole('button', { name: 'Clear', exact: true })).toBeVisible();
	await select.click({ position: { x: 5, y: 5 } });
	await page.getByRole('button', { name: 'Other server', exact: true }).click();
	await expect.element(select).toHaveTextContent('Other server');
	await page.getByRole('button', { name: 'Apply Filters' }).click();
	const applied = new URL(vi.mocked(goto).mock.calls[0][0].toString());
	expect(JSON.parse(applied.searchParams.get('mcp_server')!)).toEqual(['Other server', name]);
	expect(props.onClose).toHaveBeenCalledOnce();

	await view.unmount();
	await render(FiltersDrawer, {
		...props,
		filters: { mcp_server: applied.searchParams.get('mcp_server')! }
	});
	await expect.element(select).toHaveTextContent(name);
	await expect.element(select).toHaveTextContent('Other server');
	await select.click({ position: { x: 5, y: 5 } });
	await page.getByRole('button', { name, exact: true }).click();
	await expect.element(select).not.toHaveTextContent(name);
	await expect.element(select).toHaveTextContent('Other server');
	await select.click({ position: { x: 5, y: 5 } });
	await page.getByRole('button', { name, exact: true }).click();
	await select.getByRole('textbox').click();
	await userEvent.keyboard('{Backspace}');
	await expect.element(select).not.toHaveTextContent('Other server');
	await expect.element(select).toHaveTextContent(name);
	await select.getByRole('button').click();
	await expect.element(select).not.toHaveTextContent(name);
});
