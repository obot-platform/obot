import Table from './Table.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Row = { id: number; name: string };

// Deliberately in the order a case-sensitive SQL collation would return
// (uppercase before lowercase: "Banana" < "Zebra" < "alpha"), which is NOT
// the order a case-insensitive comparator like localeCompare would pick.
const serverOrderedRows: Row[] = [
	{ id: 1, name: 'Banana' },
	{ id: 2, name: 'Zebra' },
	{ id: 3, name: 'alpha' }
];

async function renderedNameOrder() {
	// Scoped to <tbody> only: getByRole('cell') also matches <th> (its
	// implicit role, columnheader, is a subtype of cell in the ARIA role
	// hierarchy), which would pick up the "Name" column header too.
	const cells = page.getByCSS('tbody td');
	const texts = await cells.all();
	const values: string[] = [];
	for (const cell of texts) {
		values.push((await cell.element().textContent)?.trim() ?? '');
	}
	return values;
}

describe('Table sorting', () => {
	it('re-sorts data locally by default (existing behavior for all other callers)', async () => {
		render(Table, {
			data: serverOrderedRows,
			fields: ['name'],
			headers: [{ title: 'Name', property: 'name' }],
			sortable: ['name'],
			initSort: { property: 'name', order: 'asc' }
		});

		// localeCompare treats case loosely, so this does NOT match server order.
		expect(await renderedNameOrder()).toEqual(['alpha', 'Banana', 'Zebra']);
	});

	it('leaves manualSortFields data exactly as given, trusting the caller already sorted it', async () => {
		render(Table, {
			data: serverOrderedRows,
			fields: ['name'],
			headers: [{ title: 'Name', property: 'name' }],
			sortable: ['name'],
			manualSortFields: ['name'],
			initSort: { property: 'name', order: 'asc' }
		});

		// Table must not re-sort: the order should match serverOrderedRows exactly,
		// not the localeCompare order from the previous test.
		expect(await renderedNameOrder()).toEqual(['Banana', 'Zebra', 'alpha']);
	});
});
