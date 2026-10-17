import { Group } from '$lib/services';
import { goto } from '$lib/url';
import { worker } from '../../tests/mocks/worker';
import { renderModelsTab, resetModelsTab } from './testing';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const gatewayTabs = ['Providers', 'Profiles', 'Guardrails', 'Routing', 'Settings'];

function mockProfiles(items: unknown[] = []) {
	worker.use(
		http.get('/api/model-access-policies', () => HttpResponse.json({ items })),
		http.get('/api/llm-gateway/budgets', () => HttpResponse.json({ items: [] })),
		http.get('/api/llm-gateway/guardrail-policies', () => HttpResponse.json({ items: [] }))
	);
}

function mockGuardrails() {
	worker.use(
		http.get('/api/llm-gateway/guardrails', () =>
			HttpResponse.json({ items: [{ id: 'g1', name: 'Common PII', type: 'regex', config: {} }] })
		),
		http.get('/api/llm-gateway/guardrails/providers', () =>
			HttpResponse.json({
				items: [
					{
						type: 'regex',
						label: 'Custom Regex',
						description: '',
						category: 'Built-in',
						fields: [],
						redacts: true
					}
				]
			})
		),
		http.get('/api/llm-gateway/guardrail-policies', () =>
			HttpResponse.json({
				items: [{ id: 'r1', name: 'Block PII', guardrailIDs: ['g1'], profileID: 'p-eng' }]
			})
		),
		http.get('/api/mcp-webhook-validations', () => HttpResponse.json({ items: [] }))
	);
}

afterEach(resetModelsTab);

describe('Models page', () => {
	it('shows admins the Connect tab and every gateway tab', async () => {
		await renderModelsTab('connect');

		await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'LLM Gateway' }).first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Models', exact: true }))
			.not.toBeInTheDocument();
		for (const tab of gatewayTabs) {
			await expect.element(page.getByRole('button', { name: tab, exact: true })).toBeVisible();
		}
	});

	it('shows users only their models, without gateway tabs', async () => {
		await renderModelsTab('connect', [Group.USER]);

		// With only the Connect tab, there is no tab bar at all.
		await expect.element(page.getByRole('heading', { name: 'LLM Gateway' }).first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Connect', exact: true }))
			.not.toBeInTheDocument();
		for (const tab of gatewayTabs) {
			await expect
				.element(page.getByRole('button', { name: tab, exact: true }))
				.not.toBeInTheDocument();
		}
	});

	it('opens the Connect tab by default', async () => {
		await renderModelsTab('');

		await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'LLM Gateway' }).first()).toBeVisible();
	});

	it('does not show intro copy on the Connect tab', async () => {
		await renderModelsTab('connect');

		await expect
			.element(page.getByText('Use the Obot LLM Gateway to call', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('offers Add Profile on the profiles tab', async () => {
		mockProfiles([{ id: 'p1', displayName: 'Engineering', subjects: [], models: [] }]);
		await renderModelsTab('profiles');

		await expect.element(page.getByRole('heading', { name: 'Engineering' })).toBeInTheDocument();
		await page.getByRole('button', { name: 'Add Profile' }).first().click();

		expect(goto).toHaveBeenCalledWith('/models/profiles/new');
	});

	it('lists the shared guardrails on the Guardrails tab, with what uses them', async () => {
		mockGuardrails();
		await renderModelsTab('guardrails');

		await expect.element(page.getByRole('cell', { name: 'Common PII' })).toBeInTheDocument();
		await expect.element(page.getByText('Applied in: Block PII', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText(/shared by the LLM gateway and MCP servers/))
			.toBeInTheDocument();

		await page.getByRole('button', { name: 'Add Guardrail' }).first().click();
		await page.getByRole('button', { name: /Custom Regex/ }).click();
		expect(goto).toHaveBeenCalledWith('/models/guardrails/new?type=regex');
	});

	it('hides Add Guardrail from read-only admins', async () => {
		mockGuardrails();
		await renderModelsTab('guardrails', [Group.USER, Group.AUDITOR]);

		await expect.element(page.getByRole('cell', { name: 'Common PII' })).toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Guardrail' }))
			.not.toBeInTheDocument();
	});

	it('hides create actions from read-only admins', async () => {
		mockProfiles();
		await renderModelsTab('profiles', [Group.USER, Group.AUDITOR]);

		await expect.element(page.getByRole('heading', { name: 'No profiles' })).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add Profile' })).not.toBeInTheDocument();
	});
});
