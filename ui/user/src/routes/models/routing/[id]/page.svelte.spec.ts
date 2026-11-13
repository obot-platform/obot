import { Group } from '$lib/services';
import type { RoutingRule } from '$lib/services/llm-gateway/gateway';
import { success } from '$lib/stores/success';
import { goto } from '$lib/url';
import { createMockProfile, preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import type { PageData } from './$types';
import RulePage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const provider = (name: string) => ({
	name,
	network_config: {},
	concurrency_and_buffer_size: { concurrency: 1000, buffer_size: 5000 }
});

const cheapRule: RoutingRule = {
	id: 'r1',
	name: 'Cheaper GPT',
	description: 'Send gpt-4o to mini',
	enabled: true,
	cel_expression: 'model == "gpt-4o"',
	priority: 1,
	scope: 'global',
	targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1 }],
	fallbacks: ['anthropic/claude-sonnet-4-5']
};

function mockProviders() {
	worker.use(
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers: [provider('openai'), provider('anthropic')], total: 2 })
		)
	);
}

async function renderRulePage(rule: RoutingRule = cheapRule, groups: string[] = [Group.ADMIN]) {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	mockProviders();
	const data = await preparePageData<PageData>({ profile: createMockProfile(groups), rule });
	return render(RulePage, { data });
}

function form() {
	return page.getByRole('form', { name: 'Edit Routing Rule' });
}

let notified: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
	notified = vi.spyOn(success, 'add');
});

afterEach(() => {
	vi.mocked(goto).mockClear();
	notified.mockRestore();
});

describe('Routing rule page', () => {
	it('updates a rule, keeping same-provider targets and parsed fallbacks', async () => {
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
		await renderRulePage();

		await expect.element(form().getByLabelText('Name')).toHaveValue('Cheaper GPT');
		await form().getByLabelText('Priority').fill('7');
		await form().getByRole('button', { name: 'Disable Rule' }).click();
		await form().getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(notified).toHaveBeenCalledWith('Routing rule saved.'));
		expect(goto).not.toHaveBeenCalled();
		await expect.element(form().getByLabelText('Priority')).toHaveValue(7);
		expect(updated).toHaveBeenCalledOnce();
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

	it('keeps an existing time to first token timeout and key pin', async () => {
		const rule: RoutingRule = {
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
		await renderRulePage(rule);

		await expect.element(form().getByLabelText('Time to first token timeout')).toHaveValue(1.5);
		await form().getByLabelText('Name').fill('Cheaper GPT, fast start');
		await form().getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(goto).not.toHaveBeenCalled();
		await vi.waitFor(() => expect(notified).toHaveBeenCalledWith('Routing rule saved.'));
		expect(updated.mock.calls[0][1].targets).toEqual([
			{ provider: 'openai', model: 'gpt-4o-mini', weight: 1, key_id: 'k1', ttft_timeout_ms: 1500 }
		]);
	});

	it('drops a cleared time to first token timeout', async () => {
		const rule: RoutingRule = {
			...cheapRule,
			targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1, ttft_timeout_ms: 1500 }]
		};
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
		await renderRulePage(rule);

		await form().getByLabelText('Time to first token timeout').clear();
		await form().getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(goto).not.toHaveBeenCalled();
		await vi.waitFor(() => expect(notified).toHaveBeenCalledWith('Routing rule saved.'));
		expect(updated.mock.calls[0][1].targets).toEqual([
			{ provider: null, model: 'gpt-4o-mini', weight: 1 }
		]);
	});

	it('shows the server error when saving fails', async () => {
		worker.use(
			http.put('/api/llm-gateway/admin/governance/routing-rules/:id', () =>
				HttpResponse.json({ error: { message: 'invalid CEL expression' } }, { status: 400 })
			)
		);
		await renderRulePage();

		await form().getByRole('button', { name: 'Save', exact: true }).click();

		await expect.element(page.getByText('invalid CEL expression')).toBeInTheDocument();
		expect(goto).not.toHaveBeenCalled();
		expect(notified).not.toHaveBeenCalled();
	});

	it('returns to the routing list on cancel', async () => {
		await renderRulePage();

		await form().getByRole('button', { name: 'Cancel' }).click();

		expect(goto).toHaveBeenCalledWith('/models?view=routing');
	});

	it('deletes a rule after confirmation', async () => {
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/llm-gateway/admin/governance/routing-rules/:id', ({ params }) => {
				deleted(params.id);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderRulePage();

		await expect.element(form().getByRole('heading', { name: 'Cheaper GPT' })).toBeVisible();
		await form().getByRole('button', { name: 'Delete rule' }).click();
		await expect.element(page.getByText('Delete routing rule Cheaper GPT?')).toBeInTheDocument();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('r1'));
		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models?view=routing'));
	});

	it('stays on the rule when deleting fails', async () => {
		worker.use(
			http.delete('/api/llm-gateway/admin/governance/routing-rules/:id', () =>
				HttpResponse.json({ error: { message: 'rule is in use' } }, { status: 409 })
			)
		);
		await renderRulePage();

		await form().getByRole('button', { name: 'Delete rule' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await expect.element(page.getByText('rule is in use')).toBeInTheDocument();
		expect(goto).not.toHaveBeenCalled();
	});

	it('hides saving from auditors', async () => {
		await renderRulePage(cheapRule, [Group.USER, Group.AUDITOR]);

		await expect.element(form().getByRole('heading', { name: 'Cheaper GPT' })).toBeVisible();
		await expect.element(form().getByLabelText('Name')).toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Disable Rule' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Delete rule' })).not.toBeInTheDocument();
	});
});
