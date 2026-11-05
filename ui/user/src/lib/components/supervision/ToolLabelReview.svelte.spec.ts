import { Group } from '$lib/services';
import type { ToolLabelSuggestions } from '$lib/services/provenance';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ToolLabelReview from './ToolLabelReview.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const resource = { resourceType: 'catalog-entry' as const, resourceID: 'clinical-tools' };
const suggestions: ToolLabelSuggestions = {
	...resource,
	displayName: 'Clinical tools',
	sourceURL: 'https://github.com/example/clinical-tools',
	items: [
		{
			server: 'catalog-entry:clinical-tools',
			tool: 'read_patient',
			description: 'Read patient records',
			labels: ['reads-private'],
			source: 'heuristic',
			stale: false,
			proposal: {
				labels: ['reads-private', 'credential-bearing'],
				rationale: 'This tool also returns an access token.',
				schemaHash: 'schema-read-v1',
				generatedAt: '2026-10-30T10:00:00Z'
			}
		},
		{
			server: 'catalog-entry:clinical-tools',
			tool: 'send_summary',
			labels: [],
			source: 'heuristic',
			stale: false,
			proposal: {
				labels: ['writes-external'],
				rationale: 'Can send a report to an arbitrary recipient.',
				schemaHash: 'schema-send-v1',
				generatedAt: '2026-10-30T10:00:00Z'
			}
		}
	]
};

async function renderReview(groups = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	return render(ToolLabelReview, resource);
}
function mockRead(value = suggestions) {
	worker.use(
		http.get('/api/provenance/tool-label-suggestions', ({ request }) => {
			const url = new URL(request.url);
			expect(url.searchParams.get('resourceType')).toBe('catalog-entry');
			expect(url.searchParams.get('resourceID')).toBe('clinical-tools');
			return HttpResponse.json(value);
		})
	);
}

describe('Tool label review', () => {
	it('shows the Git source and exact proposed additions without applying anything', async () => {
		mockRead();
		const applied = vi.fn();
		worker.use(http.post('/api/provenance/tool-label-suggestions/approve', applied));
		await renderReview();
		await expect.element(page.getByRole('heading', { name: 'Clinical tools' })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: suggestions.sourceURL }))
			.toHaveAttribute('href', suggestions.sourceURL);
		await expect.element(page.getByText('Add: credential-bearing')).toBeVisible();
		await expect.element(page.getByText('This tool also returns an access token.')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Approve selected (0)' })).toBeDisabled();
		expect(applied).not.toHaveBeenCalled();
	});
	it('approves only selected proposals and their reviewed schema hashes', async () => {
		mockRead();
		const applied = vi.fn();
		worker.use(
			http.post('/api/provenance/tool-label-suggestions/approve', async ({ request }) => {
				applied(await request.json());
				return HttpResponse.json({
					...suggestions,
					items: suggestions.items.map((item) => ({
						...item,
						labels: item.proposal!.labels,
						source: 'admin'
					}))
				});
			})
		);
		await renderReview();
		await page.getByRole('checkbox', { name: 'Select read_patient' }).click();
		await page.getByRole('button', { name: 'Approve selected (1)' }).click();
		await vi.waitFor(() =>
			expect(applied).toHaveBeenCalledWith({
				...resource,
				tools: [{ tool: 'read_patient', schemaHash: 'schema-read-v1' }]
			})
		);
		await expect
			.element(page.getByRole('status'))
			.toHaveTextContent('Selected tool labels approved.');
		await expect.element(page.getByRole('button', { name: 'Approve selected (0)' })).toBeDisabled();
	});
	it('generates new suggestions only when requested', async () => {
		mockRead({
			...suggestions,
			items: suggestions.items.map((item) => ({ ...item, proposal: undefined }))
		});
		const generated = vi.fn();
		worker.use(
			http.post('/api/provenance/tool-label-suggestions', async ({ request }) => {
				generated(await request.json());
				return HttpResponse.json(suggestions);
			})
		);
		await renderReview();
		await expect.element(page.getByText('No suggestion yet').first()).toBeVisible();
		expect(generated).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Suggest labels' }).click();
		await vi.waitFor(() => expect(generated).toHaveBeenCalledWith(resource));
		await expect.element(page.getByText('Add: credential-bearing')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Approve selected (0)' })).toBeDisabled();
	});
	it('keeps stale suggestions unselectable and refreshes on a conflict', async () => {
		mockRead();
		worker.use(
			http.post('/api/provenance/tool-label-suggestions/approve', () => {
				mockRead({
					...suggestions,
					items: suggestions.items.map((item) => ({ ...item, stale: true }))
				});
				return HttpResponse.json({ message: 'tools changed' }, { status: 409 });
			})
		);
		await renderReview();
		await page.getByRole('checkbox', { name: 'Select read_patient' }).click();
		await page.getByRole('button', { name: 'Approve selected (1)' }).click();
		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'The saved tools or suggestions changed. Review the refreshed list and generate new suggestions before approving.'
			);
		await expect
			.element(page.getByRole('checkbox', { name: 'Select read_patient' }))
			.toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Approve selected (0)' })).toBeDisabled();
	});
	it('lets auditors inspect suggestions without generation or approval controls', async () => {
		mockRead();
		await renderReview([Group.USER, Group.AUDITOR]);
		await expect.element(page.getByText('Add: credential-bearing')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Suggest labels' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('checkbox')).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: /Approve selected/ }))
			.not.toBeInTheDocument();
	});
	it('explains when saved tool previews are missing', async () => {
		mockRead({ ...suggestions, items: [] });
		await renderReview();
		await expect.element(page.getByText(/No saved tools are available/)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Suggest labels' })).toBeDisabled();
	});
});
