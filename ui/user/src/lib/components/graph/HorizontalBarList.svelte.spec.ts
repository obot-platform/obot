import HorizontalBarList from './HorizontalBarList.svelte';
import { createRawSnippet, type Component, type Snippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Row = { name: string; count: number };

type TooltipItem = { label: string; value: number; row: Row };

const tooltipContent: Snippet<[TooltipItem]> = createRawSnippet((item: () => TooltipItem) => ({
	render: () => `<span>${item().label} calls</span>`
}));

// vitest's render() does not infer Svelte component generics.
const List = HorizontalBarList as Component<{
	data: Row[];
	labelKey: keyof Row & string;
	valueKey: keyof Row & string;
	formatValue?: (value: number) => string;
	tooltipContent?: Snippet<[TooltipItem]>;
	class?: string;
}>;

const rows: Row[] = [
	{ name: 'Alpha', count: 10 },
	{ name: 'Beta', count: 40 },
	{ name: 'Gamma', count: 20 }
];

function renderList(data: Row[], klass?: string) {
	return render(List, {
		data,
		labelKey: 'name',
		valueKey: 'count',
		formatValue: (value: number) => String(Math.round(value)),
		tooltipContent,
		class: klass
	});
}

async function barHeight(name: string) {
	const bar = await page.getByRole('progressbar', { name }).element();
	return bar.getBoundingClientRect().height;
}

describe('HorizontalBarList.svelte', () => {
	it('renders each label and formatted value', async () => {
		renderList(rows);

		for (const row of rows) {
			await expect.element(page.getByText(row.name)).toBeVisible();
			await expect.element(page.getByText(String(row.count))).toBeVisible();
		}
	});

	it('fills each bar against the largest value in the list', async () => {
		renderList(rows);

		await expect
			.element(page.getByRole('progressbar', { name: 'Alpha' }))
			.toHaveAttribute('value', '10');
		await expect
			.element(page.getByRole('progressbar', { name: 'Beta' }))
			.toHaveAttribute('value', '40');
		await expect
			.element(page.getByRole('progressbar', { name: 'Gamma' }))
			.toHaveAttribute('value', '20');
		for (const name of ['Alpha', 'Beta', 'Gamma']) {
			await expect.element(page.getByRole('progressbar', { name })).toHaveAttribute('max', '40');
		}
	});

	it('shows the tooltip snippet while a row is hovered', async () => {
		renderList(rows);

		await page.getByRole('progressbar', { name: 'Alpha' }).hover();

		await expect.element(page.getByText('Alpha calls')).toBeVisible();
	});

	it('keeps a single bar 12px tall inside a tall container', async () => {
		renderList([rows[0]], 'h-96');

		expect(await barHeight('Alpha')).toBe(12);
	});

	it('keeps every bar 12px tall when several rows share a tall container', async () => {
		renderList(rows, 'h-96');

		for (const name of ['Alpha', 'Beta', 'Gamma']) {
			expect(await barHeight(name)).toBe(12);
		}
	});
});
