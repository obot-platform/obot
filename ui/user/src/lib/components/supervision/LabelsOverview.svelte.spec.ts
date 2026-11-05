import '$lib/services';
import { CAPABILITY_LABELS, CONTENT_LABELS, type LabelInventory } from '$lib/services/provenance';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import LabelsOverview from './LabelsOverview.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const inventory: LabelInventory = {
	resources: [
		{
			resourceType: 'catalog-entry',
			resourceID: 'clinical-tools',
			displayName: 'Clinical records',
			sourceURL: 'https://github.com/example/clinical',
			toolCount: 3,
			reviewedToolCount: 1,
			pendingToolCount: 1,
			staleToolCount: 1,
			labels: ['reads-private', 'credential-bearing']
		},
		{
			resourceType: 'vmcp',
			resourceID: 'support-tools',
			displayName: 'Support workspace',
			toolCount: 2,
			reviewedToolCount: 2,
			pendingToolCount: 0,
			staleToolCount: 0,
			labels: ['reads-untrusted', 'writes-external']
		}
	],
	observedTools: [
		{
			server: 'old-crm',
			tool: 'legacy_query',
			labels: ['reads-private'],
			source: 'supervisor',
			rationale: 'The model described a private database query.',
			updatedAt: '2026-10-30T10:00:00Z'
		},
		{
			server: 'vault',
			tool: 'read_token',
			labels: ['credential-bearing'],
			source: 'admin',
			updatedAt: '2026-10-30T10:00:00Z'
		},
		{
			server: 'web',
			tool: 'fetch',
			labels: ['reads-untrusted'],
			source: 'heuristic',
			updatedAt: '0001-01-01T00:00:00Z'
		}
	],
	capabilities: CAPABILITY_LABELS,
	contentLabels: CONTENT_LABELS,
	contentTagging: { enabled: false },
	supervisorEnabled: true
};
function mockInventory(value = inventory) {
	worker.use(http.get('/api/provenance/label-inventory', () => HttpResponse.json(value)));
}
async function renderOverview() {
	await preparePageData();
	return render(LabelsOverview);
}

describe('Labels overview', () => {
	it('shows effective assignments and separate review counts with contextual management links', async () => {
		mockInventory();
		const mutation = vi.fn();
		worker.use(
			http.post('/api/provenance/tool-label-suggestions', mutation),
			http.post('/api/provenance/tool-label-suggestions/approve', mutation)
		);
		await renderOverview();
		const clinical = page.getByRole('row').filter({ hasText: 'Clinical records' });
		await expect.element(clinical.getByText('MCP · Git-managed')).toBeVisible();
		await expect.element(clinical.getByText('1 pending')).toBeVisible();
		await expect.element(clinical.getByText('1 outdated')).toBeVisible();
		await expect.element(clinical.getByText('1 / 3')).toBeVisible();
		await expect
			.element(clinical.getByRole('link', { name: 'Manage labels for Clinical records' }))
			.toHaveAttribute('href', '/tool-labels?resourceType=catalog-entry&resourceID=clinical-tools');
		const support = page.getByRole('row').filter({ hasText: 'Support workspace' });
		await expect.element(support.getByText('vMCP · UI-managed')).toBeVisible();
		await expect
			.element(support.getByRole('link'))
			.toHaveAttribute('href', '/tool-labels?resourceType=vmcp&resourceID=support-tools');
		expect(mutation).not.toHaveBeenCalled();
	});
	it('searches effective labels and filters pending or outdated suggestions', async () => {
		mockInventory();
		await renderOverview();
		await page
			.getByRole('searchbox', { name: 'Search names or labels' })
			.fill('credential-bearing');
		await expect.element(page.getByText('Clinical records', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Support workspace', { exact: true }))
			.not.toBeInTheDocument();
		await page.getByRole('searchbox').fill('');
		await page.getByRole('checkbox', { name: 'Suggestions to review only' }).click();
		await expect.element(page.getByText('Clinical records', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Support workspace', { exact: true }))
			.not.toBeInTheDocument();
		await page.getByRole('searchbox').fill('no-matching-label');
		await expect
			.element(page.getByText('No labels or resources match these filters.'))
			.toBeVisible();
	});
	it('explains all capability and content labels and the disabled optional tagger', async () => {
		mockInventory();
		await renderOverview();
		await page.getByRole('tab', { name: 'Label library' }).click();
		const capability = page.getByRole('region', { name: 'Tool capability labels' });
		for (const label of CAPABILITY_LABELS)
			await expect.element(capability.getByText(label, { exact: true })).toBeVisible();
		const content = page.getByRole('region', { name: 'Sensitivity and trust labels' });
		for (const label of CONTENT_LABELS)
			await expect.element(content.getByText(label, { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('region', { name: 'AI content tagger' }).getByText('Configured off'))
			.toBeVisible();
		await expect
			.element(page.getByText(/AI suggestions are advisory until an administrator approves them/))
			.toBeVisible();
	});
	it('shows the configured asynchronous tagger state without offering mutations', async () => {
		mockInventory({ ...inventory, contentTagging: { enabled: true, mode: 'async' } });
		await renderOverview();
		await page.getByRole('tab', { name: 'Label library' }).click();
		await expect.element(page.getByText('Configured on')).toBeVisible();
		await expect
			.element(
				page.getByText('AI labeling runs asynchronously. New labels inform later decisions.')
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Approve|Suggest labels/ }))
			.not.toBeInTheDocument();
	});
	it('separates legacy AI observations from administrator-reviewed records and shows rationale', async () => {
		mockInventory();
		await renderOverview();
		await page.getByRole('tab', { name: 'Observed tools' }).click();
		const legacy = page.getByRole('row').filter({ hasText: 'legacy_query' });
		await expect.element(legacy.getByText('AI (legacy record)')).toBeVisible();
		await expect
			.element(legacy.getByText('The model described a private database query.'))
			.toBeVisible();
		await expect
			.element(legacy.getByText('Administrator', { exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('row').filter({ hasText: 'read_token' }).getByText('Administrator'))
			.toBeVisible();
		await expect
			.element(page.getByRole('row').filter({ hasText: 'fetch' }).getByText('Not recorded'))
			.toBeVisible();
		await page.getByRole('searchbox').fill('credential-bearing');
		await expect.element(page.getByText('read_token', { exact: true })).toBeVisible();
		await expect.element(page.getByText('legacy_query', { exact: true })).not.toBeInTheDocument();
	});
	it('has useful empty states while retaining the label library', async () => {
		mockInventory({ ...inventory, resources: [], observedTools: [] });
		await renderOverview();
		await expect
			.element(page.getByText(/No resources with saved tools or label records/))
			.toBeVisible();
		await page.getByRole('tab', { name: 'Observed tools' }).click();
		await expect
			.element(page.getByText('No observed tool label records are available yet.'))
			.toBeVisible();
		await page.getByRole('tab', { name: 'Label library' }).click();
		await expect.element(page.getByText('reads-private', { exact: true })).toBeVisible();
	});
	it('retries inventory errors without generating labels', async () => {
		worker.use(
			http.get('/api/provenance/label-inventory', () =>
				HttpResponse.json({ message: 'Inventory unavailable' }, { status: 503 })
			)
		);
		await renderOverview();
		await expect.element(page.getByRole('alert')).toBeVisible();
		mockInventory();
		await page.getByRole('button', { name: 'Retry' }).click();
		await expect.element(page.getByText('Clinical records', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	});
});

it('keeps an invalid resource visible with an actionable error without hiding healthy resources', async () => {
	mockInventory({
		...inventory,
		resources: [
			{
				...inventory.resources[0],
				toolCount: 0,
				reviewedToolCount: 0,
				pendingToolCount: 0,
				staleToolCount: 0,
				labels: [],
				inventoryError: 'Tool name contains unsupported characters.'
			},
			inventory.resources[1]
		]
	});
	await renderOverview();
	const invalid = page.getByRole('row').filter({ hasText: 'Clinical records' });
	await expect
		.element(invalid.getByText('Tool name contains unsupported characters.'))
		.toBeVisible();
	await expect.element(invalid.getByText('Inventory needs attention')).toBeVisible();
	await expect
		.element(invalid.getByRole('link', { name: 'Open resource' }))
		.toHaveAttribute('href', '/mcp-servers/c/clinical-tools?view=tools');
	await expect.element(invalid.getByText('No pending suggestions')).not.toBeInTheDocument();
	await expect
		.element(page.getByRole('link', { name: 'Manage labels for Support workspace' }))
		.toBeVisible();
});
