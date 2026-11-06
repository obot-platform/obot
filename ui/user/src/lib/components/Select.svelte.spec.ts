import Select from './Select.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function options(count: number) {
	return Array.from({ length: count }, (_, i) => ({ id: `${i + 1}`, label: `Option ${i + 1}` }));
}

describe('Select', () => {
	// A closed dropdown would otherwise keep every option in the page, which makes a page with
	// thousands of options, such as a filter of user emails, slow.
	it('renders its options only while the dropdown is open', async () => {
		render(Select, { options: options(3), placeholder: 'Pick one' });

		await expect.element(page.getByRole('combobox', { name: 'Pick one' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Option 1' })).not.toBeInTheDocument();

		await page.getByRole('combobox', { name: 'Pick one' }).click();

		await expect.element(page.getByRole('button', { name: 'Option 1' })).toBeVisible();
		await expect
			.element(page.getByRole('combobox', { name: 'Pick one' }))
			.toHaveAttribute('aria-expanded', 'true');
	});

	it('selects an option', async () => {
		const onSelect = vi.fn();
		render(Select, { options: options(3), placeholder: 'Pick one', onSelect });

		await page.getByRole('combobox', { name: 'Pick one' }).click();
		await page.getByRole('button', { name: 'Option 2' }).click();

		expect(onSelect).toHaveBeenCalledWith({ id: '2', label: 'Option 2' }, '2');
	});

	it('renders at most maxRenderedOptions, and asks to narrow the search for the rest', async () => {
		render(Select, {
			options: options(150),
			placeholder: 'Pick one',
			searchInDropdown: true,
			maxRenderedOptions: 100
		});

		await page.getByRole('combobox', { name: 'Pick one' }).click();

		await expect
			.element(page.getByRole('button', { name: 'Option 100', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Option 101', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('50 more — type to narrow the results')).toBeVisible();

		await page.getByPlaceholder('Pick one').fill('Option 15');

		await expect
			.element(page.getByRole('button', { name: 'Option 150', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(/more — type to narrow the results/))
			.not.toBeInTheDocument();
	});

	// "Option 1" is part of 61 other labels and comes last, so without ranking it first it would
	// never be within the cap however it is searched for.
	it('lists an exact match first when more options match than the cap', async () => {
		render(Select, {
			options: options(150).reverse(),
			placeholder: 'Pick one',
			searchInDropdown: true,
			maxRenderedOptions: 5
		});

		await page.getByRole('combobox', { name: 'Pick one' }).click();
		await page.getByPlaceholder('Pick one').fill('Option 1');

		await expect.element(page.getByRole('button', { name: 'Option 1', exact: true })).toBeVisible();
		await expect.element(page.getByText('57 more — type to narrow the results')).toBeVisible();
	});

	// Options past the cap are only reachable by searching, so a Select without a search input
	// renders all of them.
	it('ignores maxRenderedOptions when there is no search input', async () => {
		render(Select, { options: options(150), placeholder: 'Pick one', maxRenderedOptions: 100 });

		await page.getByRole('combobox', { name: 'Pick one' }).click();

		await expect
			.element(page.getByRole('button', { name: 'Option 150', exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/more — type to narrow the results/))
			.not.toBeInTheDocument();
	});

	it('renders every option without maxRenderedOptions', async () => {
		render(Select, { options: options(150), placeholder: 'Pick one' });

		await page.getByRole('combobox', { name: 'Pick one' }).click();

		await expect
			.element(page.getByRole('button', { name: 'Option 150', exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/more — type to narrow the results/))
			.not.toBeInTheDocument();
	});
});
