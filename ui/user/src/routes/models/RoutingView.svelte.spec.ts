import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { worker } from '../../tests/mocks/worker';
import { readyStatus, renderModelsTab, resetModelsTab } from './testing';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page, type Locator } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

const provider = (name: string) => ({
	name,
	network_config: {},
	concurrency_and_buffer_size: { concurrency: 1000, buffer_size: 5000 }
});

const cheapRule = {
	id: 'r1',
	name: 'Cheaper GPT',
	description: 'Send gpt-4o to mini',
	enabled: true,
	cel_expression: 'model == "gpt-4o"',
	priority: 1,
	scope: 'global',
	targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1 }],
	// Bifrost returns unpinned fallbacks as "provider/model" strings.
	fallbacks: ['anthropic/claude-sonnet-4-5']
};

function mockGateway({
	status = readyStatus,
	rules = [cheapRule] as unknown[]
}: { status?: typeof readyStatus; rules?: unknown[] } = {}) {
	worker.use(
		http.get('/api/llm-gateway/status', () => HttpResponse.json(status)),
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers: [provider('openai'), provider('anthropic')], total: 2 })
		),
		http.get('/api/llm-gateway/admin/governance/routing-rules', () =>
			HttpResponse.json({ rules, count: rules.length })
		)
	);
}

async function renderRoutingPage(groups: string[] = [Group.ADMIN]) {
	return renderModelsTab('routing', groups);
}

async function chooseProvider(scope: Locator, name: string, option: string) {
	await scope.getByRole('combobox', { name }).last().click();
	await page.getByRole('button', { name: option, exact: true }).click();
}

// The search box syncs its query to the URL. Add Rule and row clicks navigate through goto,
// which the specs apply onto the mocked page URL.
vi.mock(import('$lib/url'), async (importOriginal) => {
	const original = await importOriginal();
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		...original,
		replaceState: vi.fn(),
		goto: (url: Parameters<typeof original.goto>[0]) => applyTestGoto(String(url))
	};
});

afterEach(resetModelsTab);

describe('Models routing tab', () => {
	it('lists rules with their targets, fallbacks and status', async () => {
		mockGateway();
		await renderRoutingPage();

		await expect.element(page.getByText('Cheaper GPT')).toBeInTheDocument();
		await expect.element(page.getByText('same provider / gpt-4o-mini (100%)')).toBeInTheDocument();
		await expect.element(page.getByText('anthropic/claude-sonnet-4-5')).toBeInTheDocument();
		await expect
			.element(page.getByRole('cell', { name: 'Enabled', exact: true }))
			.toBeInTheDocument();
	});

	it('shows an empty state without rules', async () => {
		mockGateway({ rules: [] });
		await renderRoutingPage();

		await expect
			.element(page.getByRole('heading', { name: 'No routing rules' }))
			.toBeInTheDocument();
	});

	it('does not show the create form from a direct link while the gateway is not ready', async () => {
		mockGateway({ status: { ...readyStatus, ready: false } });
		(appPage.url as URL).searchParams.set('new', 'true');
		await renderRoutingPage();

		await expect.element(page.getByText('The LLM gateway is starting…')).toBeVisible();
		await expect
			.element(page.getByRole('form', { name: 'Add Routing Rule' }))
			.not.toBeInTheDocument();
	});

	it('creates a rule with the exact targets and fallbacks entered', async () => {
		mockGateway({ rules: [] });
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/admin/governance/routing-rules', async ({ request }) => {
				created(await request.json());
				return HttpResponse.json({ rule: { id: 'new' } });
			})
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Add Rule' }).click();
		const form = page.getByRole('form', { name: 'Add Routing Rule' });
		await form.getByLabelText('Name').fill('Split Claude');
		await form.getByLabelText('Priority').fill('3');
		await expect
			.element(
				form.getByText(
					'Lower numbers run first. Rules with equal priority have no guaranteed order; use different priorities when order matters.'
				)
			)
			.toBeVisible();
		await form.getByLabelText('When (CEL expression)').fill('model.startsWith("claude-")');
		// Enable/Disable and delete are only offered once the rule has been saved.
		await expect
			.element(form.getByRole('button', { name: 'Disable Rule' }))
			.not.toBeInTheDocument();
		await expect.element(form.getByRole('button', { name: 'Delete rule' })).not.toBeInTheDocument();
		await expect.element(form.getByText('request_type', { exact: true })).toBeInTheDocument();

		// The first target defaults to the first provider with weight 1.
		await form.getByLabelText('Target model').fill('claude-sonnet-4-5');
		await form.getByLabelText('Target weight').fill('0.75');
		await form.getByRole('button', { name: 'Add target' }).click();
		await chooseProvider(form, 'Target provider', 'Anthropic');
		await form.getByLabelText('Target weight').last().fill('0.25');

		await form.getByRole('button', { name: 'Add fallback' }).click();
		await chooseProvider(form, 'Fallback provider', 'Anthropic');
		await form.getByLabelText('Fallback model').fill('claude-haiku-4-5');
		// A fallback without a model is dropped from the payload.
		await form.getByRole('button', { name: 'Add fallback' }).click();

		await form.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith({
			name: 'Split Claude',
			description: '',
			enabled: true,
			cel_expression: 'model.startsWith("claude-")',
			priority: 3,
			scope: 'global',
			targets: [
				{ provider: 'openai', model: 'claude-sonnet-4-5', weight: 0.75 },
				{ provider: 'anthropic', model: null, weight: 0.25 }
			],
			fallbacks: [{ provider: 'anthropic', model: 'claude-haiku-4-5' }]
		});
	});

	it('does not allow saving when target weights do not add up to 1', async () => {
		mockGateway({ rules: [] });
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Add Rule' }).click();
		const form = page.getByRole('form', { name: 'Add Routing Rule' });
		await form.getByLabelText('Name').fill('Bad weights');
		await form.getByLabelText('Target weight').fill('0.5');

		await expect
			.element(form.getByText('Weights must add up to 1 (currently 0.50)'))
			.toBeInTheDocument();
		await expect.element(form.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('opens a rule from its row', async () => {
		mockGateway();
		await renderRoutingPage();
		await expect
			.element(page.getByRole('link', { name: 'Cheaper GPT' }))
			.toHaveAttribute('href', '/models/routing/r1');
		await page.getByRole('cell', { name: 'model == "gpt-4o"' }).click();

		await vi.waitFor(() => expect(appPage.url.pathname).toBe('/models/routing/r1'));
	});

	it('sends a time to first token timeout in milliseconds and omits empty ones', async () => {
		mockGateway({ rules: [] });
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/admin/governance/routing-rules', async ({ request }) => {
				created(await request.json());
				return HttpResponse.json({ rule: { id: 'new' } });
			})
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Add Rule' }).click();
		const form = page.getByRole('form', { name: 'Add Routing Rule' });
		await form.getByLabelText('Name').fill('Fast first token');
		await form.getByLabelText('Target model').fill('gpt-4o');
		await form.getByLabelText('Target weight').fill('0.5');
		await form.getByLabelText('Time to first token timeout').fill('2.5');
		await expect
			.element(form.getByText('only takes effect when there is a fallback', { exact: false }))
			.toBeInTheDocument();
		// A second target left without a timeout sends none.
		await form.getByRole('button', { name: 'Add target' }).click();
		await form.getByLabelText('Target weight').last().fill('0.5');
		await form.getByRole('button', { name: 'Add fallback' }).click();
		await chooseProvider(form, 'Fallback provider', 'Anthropic');
		await form.getByLabelText('Fallback model').fill('claude-haiku-4-5');

		await form.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created.mock.calls[0][0].targets).toEqual([
			{ provider: 'openai', model: 'gpt-4o', weight: 0.5, ttft_timeout_ms: 2500 },
			{ provider: 'openai', model: null, weight: 0.5 }
		]);
	});

	it('does not allow saving a time to first token timeout above the gateway limit', async () => {
		mockGateway({ rules: [] });
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Add Rule' }).click();
		const form = page.getByRole('form', { name: 'Add Routing Rule' });
		await form.getByLabelText('Name').fill('Too slow');
		await form.getByLabelText('Time to first token timeout').fill('301');

		await expect
			.element(
				form.getByText('Time to first token timeouts must be between 0.001 and 300 seconds.')
			)
			.toBeInTheDocument();
		await expect.element(form.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('shows a time to first token timeout in the target summary', async () => {
		const rule = {
			...cheapRule,
			targets: [
				{
					provider: 'openai',
					model: 'gpt-4o-mini',
					weight: 1,
					key_id: 'k1',
					ttft_timeout_ms: 1500
				}
			]
		};
		mockGateway({ rules: [rule] });
		await renderRoutingPage();

		await expect
			.element(page.getByText('OpenAI / gpt-4o-mini (100%, 1.5s first token timeout)'))
			.toBeInTheDocument();
	});

	it('deletes a rule after confirmation', async () => {
		mockGateway();
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/llm-gateway/admin/governance/routing-rules/:id', ({ params }) => {
				deleted(params.id);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Delete rule' }).click();
		await expect.element(page.getByText('Delete routing rule Cheaper GPT?')).toBeInTheDocument();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('r1'));
		expect(deleted).toHaveBeenCalledOnce();
	});

	it('shows a starting banner and no actions while the gateway is starting', async () => {
		mockGateway({ status: { ...readyStatus, ready: false } });
		await renderRoutingPage();

		await expect.element(page.getByText('The LLM gateway is starting…')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add Rule' })).not.toBeInTheDocument();
	});

	it('hides editing actions from auditors', async () => {
		mockGateway();
		await renderRoutingPage([Group.USER, Group.AUDITOR]);

		await expect.element(page.getByText('Cheaper GPT')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add Rule' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit rule' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Delete rule' })).not.toBeInTheDocument();
	});

	it('filters rules with the search box', async () => {
		mockGateway({
			rules: [cheapRule, { ...cheapRule, id: 'r2', name: 'Claude split', priority: 2 }]
		});
		await renderRoutingPage();

		await expect.element(page.getByText('Claude split')).toBeInTheDocument();
		await page.getByPlaceholder('Search routing rules...').fill('cheaper');

		await expect.element(page.getByText('Cheaper GPT')).toBeInTheDocument();
		await expect.element(page.getByText('Claude split')).not.toBeInTheDocument();
	});
});
