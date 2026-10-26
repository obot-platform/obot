import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import GuardrailPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const presidio = {
	type: 'presidio',
	label: 'Microsoft Presidio',
	description: 'Detects personal data with a self-hosted Presidio Analyzer.',
	category: 'Integrations',
	redacts: true,
	fields: [
		{ name: 'analyzerURL', label: 'Analyzer URL', type: 'text', required: true },
		{ name: 'entities', label: 'Entities', type: 'list' },
		{ name: 'scoreThreshold', label: 'Minimum score', type: 'number', default: 0.5 },
		{ name: 'apiKey', label: 'API key', type: 'password', sensitive: true }
	]
};

// Presidio as the server describes it: entity types default to personal data and can be
// loaded from the Analyzer.
const presidioWithChoices = {
	...presidio,
	fields: [
		{ name: 'analyzerURL', label: 'Analyzer URL', type: 'text', required: true, destination: true },
		{
			name: 'entities',
			label: 'Entities',
			type: 'list',
			default: ['EMAIL_ADDRESS', 'US_SSN'],
			optionsFromService: true
		},
		{ name: 'apiKey', label: 'API key', type: 'password', sensitive: true }
	]
};

function mockProviders() {
	worker.use(
		http.get('/api/llm-gateway/guardrails/providers', () =>
			HttpResponse.json({ items: [presidio] })
		)
	);
}

afterEach(() => {
	delete (appPage.params as Record<string, string>).id;
	(appPage.url as URL).searchParams.delete('type');
});

describe('Guardrail editor', () => {
	it('creates a guardrail from the provider fields, then enables testing', async () => {
		mockProviders();
		(appPage.url as URL).searchParams.set('type', 'presidio');
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/guardrails', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({ ...body, id: 'p1', config: {}, configuredSecrets: ['apiKey'] });
			})
		);
		await preparePageData();
		render(GuardrailPage);

		await expect
			.element(page.getByLabelText('Name', { exact: true }))
			.toHaveValue('Microsoft Presidio');
		await expect.element(page.getByText('Save the guardrail to test it.')).toBeInTheDocument();
		await expect.element(page.getByLabelText('Sample text')).toBeDisabled();

		const save = page.getByRole('button', { name: 'Save', exact: true });
		await expect.element(save).toBeDisabled();
		await page.getByLabelText(/^Analyzer URL/).fill('http://presidio:3000');
		await page.getByLabelText('Entities').fill('EMAIL_ADDRESS\nUS_SSN\n');
		await page.getByLabelText('API key').fill('secret');
		await save.click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith({
			name: 'Microsoft Presidio',
			description: '',
			type: 'presidio',
			config: {
				analyzerURL: 'http://presidio:3000',
				entities: ['EMAIL_ADDRESS', 'US_SSN'],
				scoreThreshold: 0.5,
				apiKey: 'secret'
			}
		});
		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/models/guardrails/p1', {
				replaceState: true
			})
		);
		await expect.element(page.getByLabelText('Sample text')).toBeEnabled();
	});

	it('keeps configured secrets when left blank and runs a test', async () => {
		mockProviders();
		(appPage.params as Record<string, string>).id = 'p1';
		const updated = vi.fn();
		const tested = vi.fn();
		worker.use(
			http.get('/api/llm-gateway/guardrails/p1', () =>
				HttpResponse.json({
					id: 'p1',
					name: 'PII',
					type: 'presidio',
					config: { analyzerURL: 'http://presidio:3000', entities: ['EMAIL_ADDRESS'] },
					configuredSecrets: ['apiKey']
				})
			),
			http.put('/api/llm-gateway/guardrails/p1', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				updated(body);
				return HttpResponse.json({ ...body, id: 'p1', configuredSecrets: ['apiKey'] });
			}),
			http.post('/api/llm-gateway/guardrails/p1/test', async ({ request }) => {
				tested(await request.json());
				return HttpResponse.json({
					findings: [{ category: 'EMAIL_ADDRESS', count: 2 }],
					redactedText: 'Mail [EMAIL_ADDRESS] or [EMAIL_ADDRESS]',
					canRedact: true
				});
			})
		);
		await preparePageData();
		render(GuardrailPage);

		await expect.element(page.getByLabelText('Entities')).toHaveValue('EMAIL_ADDRESS');
		await expect
			.element(page.getByPlaceholder('Configured — leave blank to keep'))
			.toBeInTheDocument();

		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated.mock.calls[0][0].config).not.toHaveProperty('apiKey');

		await page.getByRole('combobox', { name: 'Stage' }).click();
		const output = page.getByRole('button', { name: 'Output', exact: true });
		await expect.element(output).toBeVisible();
		await output.click();
		await page.getByLabelText('Sample text').fill('Mail a@example.com or b@example.com');
		await page.getByRole('button', { name: 'Run test' }).click();

		await vi.waitFor(() =>
			expect(tested).toHaveBeenCalledWith({
				target: 'llm',
				stage: 'output',
				text: 'Mail a@example.com or b@example.com'
			})
		);
		await expect.element(page.getByText('1 finding')).toBeInTheDocument();
		await expect
			.element(page.getByText('Mail [EMAIL_ADDRESS] or [EMAIL_ADDRESS]'))
			.toBeInTheDocument();
	});

	it('starts with the default entity types and saves the chosen ones', async () => {
		worker.use(
			http.get('/api/llm-gateway/guardrails/providers', () =>
				HttpResponse.json({ items: [presidioWithChoices] })
			)
		);
		(appPage.url as URL).searchParams.set('type', 'presidio');
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/guardrails', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({ ...body, id: 'p1', configuredSecrets: [] });
			})
		);
		await preparePageData();
		render(GuardrailPage);

		await expect.element(page.getByRole('checkbox', { name: 'EMAIL_ADDRESS' })).toBeChecked();
		await expect.element(page.getByRole('checkbox', { name: 'US_SSN' })).toBeChecked();
		await page.getByLabelText(/^Analyzer URL/).fill('http://presidio:3000');
		await page.getByRole('checkbox', { name: 'US_SSN' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith(
			expect.objectContaining({
				config: { analyzerURL: 'http://presidio:3000', entities: ['EMAIL_ADDRESS'] }
			})
		);
	});

	it('loads the entity types the service supports with the form settings', async () => {
		worker.use(
			http.get('/api/llm-gateway/guardrails/providers', () =>
				HttpResponse.json({ items: [presidioWithChoices] })
			),
			http.get('/api/llm-gateway/guardrails/p1', () =>
				HttpResponse.json({
					id: 'p1',
					name: 'PII',
					type: 'presidio',
					config: { analyzerURL: 'http://presidio:3000', entities: ['US_SSN'] },
					configuredSecrets: ['apiKey']
				})
			)
		);
		(appPage.params as Record<string, string>).id = 'p1';
		const requested = vi.fn();
		worker.use(
			http.post(
				'/api/llm-gateway/guardrails/providers/presidio/fields/entities/options',
				async ({ request }) => {
					requested(await request.json());
					return HttpResponse.json({ items: ['EMAIL_ADDRESS', 'EMPLOYEE_ID', 'US_SSN'] });
				}
			)
		);
		await preparePageData();
		render(GuardrailPage);

		await expect.element(page.getByRole('checkbox', { name: 'US_SSN' })).toBeChecked();
		await expect
			.element(page.getByRole('checkbox', { name: 'EMPLOYEE_ID' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Load all types from the service' }).click();

		await expect.element(page.getByRole('checkbox', { name: 'EMPLOYEE_ID' })).not.toBeChecked();
		await expect.element(page.getByRole('checkbox', { name: 'US_SSN' })).toBeChecked();
		// The saved API key isn't in the form; the server uses it for the saved guardrail.
		expect(requested).toHaveBeenCalledWith({
			guardrailID: 'p1',
			config: { analyzerURL: 'http://presidio:3000', entities: ['US_SSN'] }
		});
	});

	it('shows why the entity types could not be loaded', async () => {
		worker.use(
			http.get('/api/llm-gateway/guardrails/providers', () =>
				HttpResponse.json({ items: [presidioWithChoices] })
			),
			http.post('/api/llm-gateway/guardrails/providers/presidio/fields/entities/options', () =>
				HttpResponse.text('Presidio request failed: connection refused', { status: 502 })
			)
		);
		(appPage.url as URL).searchParams.set('type', 'presidio');
		await preparePageData();
		render(GuardrailPage);

		await page.getByRole('button', { name: 'Load all types from the service' }).click();
		await expect
			.element(page.getByText('Presidio request failed: connection refused'))
			.toBeInTheDocument();
	});

	it('returns to the guardrails list on cancel', async () => {
		mockProviders();
		(appPage.url as URL).searchParams.set('type', 'presidio');
		await preparePageData();
		render(GuardrailPage);

		await expect
			.element(page.getByRole('heading', { name: 'Microsoft Presidio' }))
			.toBeInTheDocument();
		await page.getByRole('button', { name: 'Cancel' }).click();

		expect(goto).toHaveBeenCalledWith('/models?view=guardrails');
	});

	it('reports an unknown guardrail type', async () => {
		mockProviders();
		(appPage.url as URL).searchParams.set('type', 'nope');
		await preparePageData();
		render(GuardrailPage);

		await expect.element(page.getByText('Unknown guardrail type "nope".')).toBeInTheDocument();
	});
});
