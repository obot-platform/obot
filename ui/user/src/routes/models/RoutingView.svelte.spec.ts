import { Group } from '$lib/services';
import { worker } from '../../tests/mocks/worker';
import { readyStatus, renderModelsTab, resetModelsTab } from './testing';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page, type Locator } from 'vitest/browser';

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

// The search box syncs its query to the URL, which needs a router the specs don't have.
vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	replaceState: vi.fn()
}));

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
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name').fill('Split Claude');
		await dialog.getByLabelText('Priority').fill('3');
		await dialog.getByLabelText('When (CEL expression)').fill('model.startsWith("claude-")');
		// New rules start enabled, so creating one offers no toggle.
		await expect.element(dialog.getByRole('switch', { name: 'Enabled' })).not.toBeInTheDocument();
		await expect.element(dialog.getByText('request_type', { exact: true })).toBeInTheDocument();

		// The first target defaults to the first provider with weight 1.
		await dialog.getByLabelText('Target model').fill('claude-sonnet-4-5');
		await dialog.getByLabelText('Target weight').fill('0.75');
		await dialog.getByRole('button', { name: 'Add target' }).click();
		await chooseProvider(dialog, 'Target provider', 'Anthropic');
		await dialog.getByLabelText('Target weight').last().fill('0.25');

		await dialog.getByRole('button', { name: 'Add fallback' }).click();
		await chooseProvider(dialog, 'Fallback provider', 'Anthropic');
		await dialog.getByLabelText('Fallback model').fill('claude-haiku-4-5');
		// A fallback without a model is dropped from the payload.
		await dialog.getByRole('button', { name: 'Add fallback' }).click();

		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

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
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name').fill('Bad weights');
		await dialog.getByLabelText('Target weight').fill('0.5');

		await expect
			.element(dialog.getByText('Weights must add up to 1 (currently 0.50)'))
			.toBeInTheDocument();
		await expect.element(dialog.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('updates an existing rule, keeping same-provider targets and parsed fallbacks', async () => {
		mockGateway();
		const updated = vi.fn();
		worker.use(
			http.put(
				'/api/llm-gateway/admin/governance/routing-rules/:id',
				async ({ params, request }) => {
					updated(params.id, await request.json());
					return HttpResponse.json({ rule: cheapRule });
				}
			)
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Edit rule' }).click();
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByLabelText('Name')).toHaveValue('Cheaper GPT');
		await dialog.getByLabelText('Priority').fill('7');
		await dialog.getByRole('switch', { name: 'Enabled' }).click();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated).toHaveBeenCalledWith('r1', {
			name: 'Cheaper GPT',
			description: 'Send gpt-4o to mini',
			enabled: false,
			cel_expression: 'model == "gpt-4o"',
			priority: 7,
			scope: 'global',
			targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1 }],
			fallbacks: [{ provider: 'anthropic', model: 'claude-sonnet-4-5' }]
		});
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
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name').fill('Fast first token');
		await dialog.getByLabelText('Target model').fill('gpt-4o');
		await dialog.getByLabelText('Target weight').fill('0.5');
		await dialog.getByLabelText('Time to first token timeout').fill('2.5');
		await expect
			.element(dialog.getByText('only takes effect when there is a fallback', { exact: false }))
			.toBeInTheDocument();
		// A second target left without a timeout sends none.
		await dialog.getByRole('button', { name: 'Add target' }).click();
		await dialog.getByLabelText('Target weight').last().fill('0.5');
		await dialog.getByRole('button', { name: 'Add fallback' }).click();
		await chooseProvider(dialog, 'Fallback provider', 'Anthropic');
		await dialog.getByLabelText('Fallback model').fill('claude-haiku-4-5');

		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

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
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name').fill('Too slow');
		await dialog.getByLabelText('Time to first token timeout').fill('301');

		await expect
			.element(
				dialog.getByText('Time to first token timeouts must be between 0.001 and 300 seconds.')
			)
			.toBeInTheDocument();
		await expect.element(dialog.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('keeps an existing time to first token timeout and key pin when editing a rule', async () => {
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
		const updated = vi.fn();
		worker.use(
			http.put(
				'/api/llm-gateway/admin/governance/routing-rules/:id',
				async ({ params, request }) => {
					updated(params.id, await request.json());
					return HttpResponse.json({ rule });
				}
			)
		);
		await renderRoutingPage();

		await expect
			.element(page.getByText('OpenAI / gpt-4o-mini (100%, 1.5s first token timeout)'))
			.toBeInTheDocument();
		await page.getByRole('button', { name: 'Edit rule' }).click();
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByLabelText('Time to first token timeout')).toHaveValue(1.5);
		await dialog.getByLabelText('Name').fill('Cheaper GPT, fast start');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated.mock.calls[0][1].targets).toEqual([
			{ provider: 'openai', model: 'gpt-4o-mini', weight: 1, key_id: 'k1', ttft_timeout_ms: 1500 }
		]);
	});

	it('drops a cleared time to first token timeout when editing a rule', async () => {
		const rule = {
			...cheapRule,
			targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1, ttft_timeout_ms: 1500 }]
		};
		mockGateway({ rules: [rule] });
		const updated = vi.fn();
		worker.use(
			http.put(
				'/api/llm-gateway/admin/governance/routing-rules/:id',
				async ({ params, request }) => {
					updated(params.id, await request.json());
					return HttpResponse.json({ rule });
				}
			)
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Edit rule' }).click();
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Time to first token timeout').clear();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated.mock.calls[0][1].targets).toEqual([
			{ provider: null, model: 'gpt-4o-mini', weight: 1 }
		]);
	});

	it('shows the server error when saving fails', async () => {
		mockGateway();
		worker.use(
			http.put('/api/llm-gateway/admin/governance/routing-rules/:id', () =>
				HttpResponse.json({ error: { message: 'invalid CEL expression' } }, { status: 400 })
			)
		);
		await renderRoutingPage();

		await page.getByRole('button', { name: 'Edit rule' }).click();
		await page.getByRole('dialog').getByRole('button', { name: 'Save', exact: true }).click();

		await expect.element(page.getByText('invalid CEL expression')).toBeInTheDocument();
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
