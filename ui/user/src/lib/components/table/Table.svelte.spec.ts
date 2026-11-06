import Table from './Table.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Row = { id: string; name: string; kind: string };

// Row N is named so that it sorts by its number: "row 001", "row 002", ...
function rows(count: number, kind = 'a', offset = 0): Row[] {
	return Array.from({ length: count }, (_, i) => {
		const n = String(offset + i + 1).padStart(3, '0');
		return { id: `${kind}-${n}`, name: `row ${n}`, kind };
	});
}

function renderTable(props: Partial<Parameters<typeof Table<Row>>[1]> & { data: Row[] }) {
	return render(Table<Row>, {
		fields: ['name', 'kind'],
		headers: [
			{ title: 'Name', property: 'name' },
			{ title: 'Kind', property: 'kind' }
		],
		pageSize: 50,
		sortable: ['name'],
		initSort: { property: 'name', order: 'asc' },
		...props
	});
}

describe('Table pagination', () => {
	it('pages through the rows', async () => {
		renderTable({ data: rows(120) });

		await expect.element(page.getByText('1 of 3')).toBeVisible();
		await expect.element(page.getByText('row 050')).toBeVisible();
		await expect.element(page.getByText('row 051')).not.toBeInTheDocument();

		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText('2 of 3')).toBeVisible();
		await expect.element(page.getByText('row 051')).toBeVisible();
		await expect.element(page.getByText('row 050')).not.toBeInTheDocument();
	});

	it('cannot go past the last page when the rows fill it exactly', async () => {
		renderTable({ data: rows(100) });

		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText('2 of 2')).toBeVisible();
		await expect.element(page.getByText('row 100')).toBeVisible();
		await expect.element(page.getByRole('button', { name: /Next/ })).toBeDisabled();
	});

	it('counts only the rows left by the column filters', async () => {
		renderTable({
			data: [...rows(60, 'a'), ...rows(60, 'b', 60)],
			filterable: ['kind'],
			filters: { kind: ['a'] }
		});

		await expect.element(page.getByText('1 of 2')).toBeVisible();
		await page.getByRole('button', { name: /Next/ }).click();
		await expect.element(page.getByText('row 060')).toBeVisible();
		await expect.element(page.getByText('row 061')).not.toBeInTheDocument();
	});

	it('returns to the first page when the sort changes', async () => {
		renderTable({ data: rows(120) });

		await page.getByRole('button', { name: /Next/ }).click();
		await expect.element(page.getByText('2 of 3')).toBeVisible();

		await page.getByRole('columnheader', { name: 'Name' }).click();

		await expect.element(page.getByText('1 of 3')).toBeVisible();
		await expect.element(page.getByText('row 120')).toBeVisible();
	});

	async function searchKindFilter(text: string) {
		await page.getByRole('button', { name: 'Kind', exact: true }).click();
		await page.getByRole('combobox', { name: 'Filter by Kind...' }).click();
		await page.getByPlaceholder('Filter by Kind...').fill(text);
	}

	// The filter renders at most 100 options, and here every value has the same label, so the
	// labels show the values, and the last owner is reached by searching for its value.
	it('shows the values of formatted filter options that share a label', async () => {
		const data = rows(101).map((row, i) => ({ ...row, kind: `owner-${i + 1}` }));
		renderTable({
			data,
			filterable: ['kind'],
			displayValue: (property, value) => (property === 'kind' ? 'Alex Smith' : String(value))
		});

		await searchKindFilter('owner-101');
		await page.getByRole('button', { name: 'Alex Smith (owner-101)', exact: true }).click();

		await expect.element(page.getByText('row 101')).toBeVisible();
		await expect.element(page.getByText('row 001')).not.toBeInTheDocument();
	});

	// A search that matched values the options do not show would list options for no visible reason.
	it('searches formatted filter options only by what they show', async () => {
		const names: Record<string, string> = {
			'default-gmail': 'Gmail',
			'default-slack': 'Slack'
		};
		renderTable({
			data: [
				{ id: '1', name: 'row 001', kind: 'default-gmail' },
				{ id: '2', name: 'row 002', kind: 'default-slack' }
			],
			filterable: ['kind'],
			displayValue: (property, value) =>
				property === 'kind' ? names[String(value)] : String(value)
		});

		await searchKindFilter('default');
		await expect.element(page.getByText('No options available')).toBeVisible();

		await page.getByPlaceholder('Filter by Kind...').fill('gmail');
		await expect.element(page.getByRole('button', { name: 'Gmail', exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /default-/ })).not.toBeInTheDocument();
	});

	it('shows no pager when the rows fit on one page', async () => {
		renderTable({ data: rows(50) });

		await expect.element(page.getByText('row 050')).toBeVisible();
		await expect.element(page.getByRole('button', { name: /Next/ })).not.toBeInTheDocument();
	});
});
