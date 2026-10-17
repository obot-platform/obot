import { page as appPage } from '$app/state';
import { getLocale, overwriteGetLocale } from '$lib/paraglide/runtime';
import { Group } from '$lib/services';
import { goto } from '$lib/url';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import GuardrailsList from './GuardrailsList.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const providers = [
	{
		type: 'regex',
		label: 'Custom Regex',
		description: 'Matches content against regular expressions.',
		category: 'Built-in',
		fields: [],
		redacts: true
	},
	{
		type: 'presidio',
		label: 'Microsoft Presidio',
		description: 'Detects personal data with a self-hosted Presidio Analyzer.',
		category: 'Integrations',
		fields: [],
		redacts: true
	}
];

function mockGuardrails(guardrails: unknown[], policies: unknown[] = [], filters: unknown[] = []) {
	worker.use(
		http.get('/api/llm-gateway/guardrails', () => HttpResponse.json({ items: guardrails })),
		http.get('/api/llm-gateway/guardrails/providers', () =>
			HttpResponse.json({ items: providers })
		),
		http.get('/api/llm-gateway/guardrail-policies', () => HttpResponse.json({ items: policies })),
		http.get('/api/mcp-webhook-validations', () => HttpResponse.json({ items: filters })),
		http.get('/api/model-access-policies', () =>
			HttpResponse.json({
				items: [{ id: 'map-eng', displayName: 'Engineering', subjects: [], models: [] }]
			})
		)
	);
}

const regexGuardrail = { id: 'p1', name: 'Common PII', type: 'regex', config: {} };
const presidioGuardrail = {
	id: 'p2',
	name: 'Presidio',
	type: 'presidio',
	config: {},
	error: 'Analyzer URL is required'
};

async function renderView(groups: string[] = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	return render(GuardrailsList);
}

const restoreLocale = getLocale;

afterEach(() => {
	appPage.url.searchParams.delete('query');
	overwriteGetLocale(restoreLocale);
});

describe('Guardrails list', () => {
	it('lists guardrails with their type and status', async () => {
		mockGuardrails([regexGuardrail, presidioGuardrail]);
		await renderView();

		await expect.element(page.getByText('Common PII')).toBeInTheDocument();
		await expect.element(page.getByRole('cell', { name: 'Custom Regex' })).toBeInTheDocument();
		await expect.element(page.getByText('Analyzer URL is required')).toBeInTheDocument();
		await expect.element(page.getByText('Ready', { exact: true })).toBeInTheDocument();
	});

	it('translates the type column', async () => {
		overwriteGetLocale(() => 'ja');
		mockGuardrails([regexGuardrail]);
		await renderView();

		await expect.element(page.getByRole('cell', { name: 'カスタム正規表現' })).toBeInTheDocument();
		await expect.element(page.getByText('Custom Regex')).not.toBeInTheDocument();
	});

	it('says guardrails are shared and points to where they are used', async () => {
		mockGuardrails([regexGuardrail]);
		await renderView();

		await expect
			.element(page.getByText(/shared by the LLM gateway and MCP servers/))
			.toBeInTheDocument();
		await expect.element(page.getByText(/used by applied guardrails in/)).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'LLM Gateway → Profiles' }))
			.toHaveAttribute('href', '/models?view=profiles');
		await expect
			.element(page.getByRole('link', { name: 'MCP Servers → Filters' }))
			.toHaveAttribute('href', '/mcp-servers?view=filters');
	});

	it('shows the applied guardrails, with their profiles, and filters that use each guardrail', async () => {
		mockGuardrails(
			[regexGuardrail, presidioGuardrail],
			[
				{ id: 'r1', name: 'Block PII', guardrailIDs: ['p1'], profileID: 'map-eng' },
				{ id: 'r2', name: 'Mask PII', guardrailIDs: ['p1', 'p3'], profileID: 'map-eng' }
			],
			[
				{ id: 'f1', name: 'No PII in tools', guardrail: { guardrailIDs: ['p1'], action: 'block' } },
				{ id: 'f2', name: 'Webhook', url: 'https://example.com' }
			]
		);
		await renderView();

		await expect
			.element(
				page.getByText('Applied in: Engineering / Block PII, Engineering / Mask PII', {
					exact: true
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('Filters: No PII in tools', { exact: true }))
			.toBeInTheDocument();
		await expect.element(page.getByText('Not used', { exact: true })).toBeInTheDocument();
	});

	it('opens a guardrail under LLM Gateway', async () => {
		mockGuardrails([regexGuardrail]);
		await renderView();

		await page.getByText('Common PII').click();
		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models/guardrails/p1'));
	});

	it('filters guardrails by the search query', async () => {
		mockGuardrails([regexGuardrail, presidioGuardrail]);
		appPage.url.searchParams.set('query', 'presidio');
		await renderView();

		await expect
			.element(page.getByRole('cell', { name: 'Presidio', exact: true }))
			.toBeInTheDocument();
		await expect.element(page.getByText('Common PII')).not.toBeInTheDocument();
	});

	it('shows an empty state that points to where guardrails are used', async () => {
		mockGuardrails([]);
		await renderView();

		await expect.element(page.getByRole('heading', { name: 'No guardrails' })).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'LLM Gateway → Profiles' }))
			.toBeInTheDocument();
	});

	it('picks a provider type from categorized cards to add a guardrail', async () => {
		mockGuardrails([]);
		const view = await renderView();
		await expect.element(page.getByRole('heading', { name: 'No guardrails' })).toBeInTheDocument();

		view.component.openTypePicker();
		await expect.element(page.getByText('Built-in', { exact: true })).toBeInTheDocument();
		await expect
			.element(page.getByText('3rd Party Integrations', { exact: true }))
			.toBeInTheDocument();
		// Nothing is configured yet, so nothing is marked.
		await expect.element(page.getByText('Configured', { exact: true })).not.toBeInTheDocument();
		await page.getByRole('button', { name: /Microsoft Presidio/ }).click();

		expect(goto).toHaveBeenCalledWith('/models/guardrails/new?type=presidio');
	});

	it('marks integrations that are already configured, and still lets another be added', async () => {
		mockGuardrails([regexGuardrail, presidioGuardrail]);
		const view = await renderView();
		await expect.element(page.getByText('Common PII')).toBeInTheDocument();

		view.component.openTypePicker();
		const presidio = page.getByRole('button', { name: /Microsoft Presidio/ });
		const badge = presidio.getByText('Configured', { exact: true });
		await expect.element(badge).toBeVisible();
		// Built-in types are commonly used several times, so they aren't marked.
		await expect
			.element(page.getByRole('button', { name: /Custom Regex/ }).getByText('Configured'))
			.not.toBeInTheDocument();

		await badge.hover();
		await expect.element(page.getByText(/Already configured as Presidio\./)).toBeVisible();

		await expect.element(presidio).toBeEnabled();
		await presidio.click();
		expect(goto).toHaveBeenCalledWith('/models/guardrails/new?type=presidio');
	});

	it('shows why a guardrail in use cannot be deleted', async () => {
		mockGuardrails([regexGuardrail]);
		worker.use(
			http.delete('/api/llm-gateway/guardrails/p1', () =>
				HttpResponse.json(
					{ error: 'guardrail is used by policy "Block PII"; remove it from the policy first' },
					{ status: 409 }
				)
			)
		);
		await renderView();

		await page.getByRole('button', { name: 'Delete guardrail' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await expect
			.element(
				page.getByText('guardrail is used by policy "Block PII"; remove it from the policy first')
			)
			.toBeInTheDocument();
	});

	it('hides delete from auditors', async () => {
		mockGuardrails([regexGuardrail]);
		await renderView([Group.USER, Group.AUDITOR]);

		await expect.element(page.getByText('Common PII')).toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete guardrail' }))
			.not.toBeInTheDocument();
	});
});
