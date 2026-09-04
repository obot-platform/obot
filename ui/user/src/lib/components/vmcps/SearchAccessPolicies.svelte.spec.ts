import SearchAccessPolicies from '$lib/components/vmcps/SearchAccessPolicies.svelte';
import type { AccessControlRule } from '$lib/services';
import { worker } from '../../../tests/mocks/worker';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createPolicy(
	overrides: Partial<AccessControlRule> & { id: string; displayName: string }
): AccessControlRule {
	return {
		created: '2026-01-01T00:00:00Z',
		subjects: [{ type: 'user', id: 'user-1' }],
		resources: [],
		...overrides
	};
}

const engineeringPolicy = createPolicy({
	id: 'policy-engineering',
	displayName: 'Engineering Access',
	subjects: [
		{ type: 'user', id: 'user-1' },
		{ type: 'group', id: 'group-1' }
	]
});
const salesPolicy = createPolicy({
	id: 'policy-sales',
	displayName: 'Sales Access'
});
const everythingPolicy = createPolicy({
	id: 'policy-everything',
	displayName: 'Everyone Everything',
	resources: [{ type: 'selector', id: '*' }]
});

function mockAccessPolicies(items: AccessControlRule[]) {
	worker.use(
		http.get('/api/mcp-catalogs/default/access-control-rules', () => HttpResponse.json({ items }))
	);
}

async function renderDialog(
	props: Partial<{ onAdd: (policies: AccessControlRule[]) => void; filterIds: string[] }> = {}
) {
	const onAdd = props.onAdd ?? vi.fn();
	const result = await render(SearchAccessPolicies, {
		onAdd,
		filterIds: props.filterIds
	});
	await result.component.open();
	return { result, onAdd };
}

describe('SearchAccessPolicies.svelte', () => {
	it('lists existing access policies and adds the selected ones', async () => {
		mockAccessPolicies([engineeringPolicy, salesPolicy, everythingPolicy]);
		const { onAdd } = await renderDialog();

		await expect.element(page.getByText('Engineering Access', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Sales Access', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Everyone Everything', { exact: true }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Engineering Access 2 subjects' }).click();
		await page.getByRole('button', { name: 'Confirm' }).click();

		expect(onAdd).toHaveBeenCalledWith([engineeringPolicy]);
	});

	it('hides policies that are already assigned', async () => {
		mockAccessPolicies([engineeringPolicy, salesPolicy]);
		await renderDialog({ filterIds: [engineeringPolicy.id] });

		await expect.element(page.getByText('Sales Access', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Engineering Access', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('filters policies by name', async () => {
		mockAccessPolicies([engineeringPolicy, salesPolicy]);
		await renderDialog();

		await page.getByPlaceholder('Search by access policy name...').fill('sales');

		await expect.element(page.getByText('Sales Access', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Engineering Access', { exact: true }))
			.not.toBeInTheDocument();
	});
});
