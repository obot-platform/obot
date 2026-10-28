import { page as appPage } from '$app/state';
import type { LLMAuditGuardrailFinding, LLMAuditLog } from '$lib/services';
import AuditLogsPage from '../../../../routes/audit-logs/+page.svelte';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import LlmAuditLogGuardrails from './LlmAuditLogGuardrails.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const findings: LLMAuditGuardrailFinding[] = [
	{
		stage: 'input',
		policyID: 'policy-1',
		policyName: 'Mask secrets',
		guardrailName: 'Secrets',
		guardrailType: 'secrets',
		outcome: 'redacted',
		categories: ['secret']
	},
	{
		stage: 'output',
		policyID: 'policy-2',
		policyName: 'Block PII',
		guardrailName: 'presidio',
		guardrailType: 'presidio',
		outcome: 'blocked',
		categories: ['EMAIL_ADDRESS', 'PHONE_NUMBER']
	},
	{
		stage: 'output',
		policyID: 'policy-3',
		policyName: '',
		guardrailName: 'Moderation',
		guardrailType: 'lakera',
		outcome: 'error'
	}
];

function auditLog(overrides: Partial<LLMAuditLog>): LLMAuditLog {
	return {
		id: 'audit-1',
		createdAt: '2026-10-01T12:00:00Z',
		duration: 120,
		userID: 'user-1',
		modelProvider: 'llm-gateway-model-provider',
		modelID: 'm1',
		targetModel: 'openai/gpt-4o',
		reasoningEffort: '',
		requestPath: '/api/llm-proxy/v1/chat/completions',
		requestMethod: 'POST',
		requestID: 'req-1',
		responseID: '',
		responseStatus: 200,
		outcome: 'success',
		inputTokens: 10,
		outputTokens: 5,
		userAgent: 'curl/8',
		clientSessionID: '',
		clientIP: '10.0.0.1',
		...overrides
	};
}

describe('LlmAuditLogGuardrails', () => {
	it('lists what each policy did, with its stage, guardrail and categories', async () => {
		render(LlmAuditLogGuardrails, { findings });

		const section = page.getByRole('region', { name: 'Guardrails' });
		await expect.element(section).toBeVisible();
		// The content a policy found is never shown: only its outcome, stage, guardrail and categories.
		const items = section.getByRole('listitem');
		await expect
			.element(items.nth(0))
			.toHaveTextContent(
				'Redacted Mask secrets Request Guardrail: Secrets (secrets) Found: secret'
			);
		const blocked = items.nth(1);
		await expect.element(blocked.getByText('Blocked', { exact: true })).toBeVisible();
		await expect.element(blocked.getByText('Block PII', { exact: true })).toBeVisible();
		await expect.element(blocked.getByText('Response', { exact: true })).toBeVisible();
		await expect.element(blocked.getByText('EMAIL_ADDRESS', { exact: true })).toBeVisible();
		await expect.element(blocked.getByText('PHONE_NUMBER', { exact: true })).toBeVisible();
		// A policy without a name is shown by its ID, and a guardrail error has no categories.
		await expect
			.element(items.nth(2))
			.toHaveTextContent('Error policy-3 Response Guardrail: Moderation (lakera)');
	});
});

describe('Model audit logs guardrail findings', () => {
	afterEach(() => {
		appPage.url.searchParams.delete('view');
	});

	function mockAuditLogs() {
		const blocked = auditLog({
			id: 'audit-blocked',
			requestID: 'req-blocked',
			responseStatus: 400,
			outcome: 'error',
			guardrailFindings: findings
		});
		const clean = auditLog({ id: 'audit-clean', requestID: 'req-clean', userAgent: 'clean-agent' });
		const details = vi.fn();
		worker.use(
			http.get('/api/llm-audit-logs', () =>
				HttpResponse.json({ items: [blocked, clean], total: 2, limit: 100, offset: 0 })
			),
			http.get('/api/llm-audit-logs/detail/:id', ({ params }) => {
				details(params.id);
				return HttpResponse.json(params.id === blocked.id ? blocked : clean);
			})
		);
		return { details };
	}

	it('marks requests a guardrail acted on and lists the findings in the details', async () => {
		const { details } = mockAuditLogs();
		appPage.url.searchParams.set('view', 'llm');
		await preparePageData();
		render(AuditLogsPage);

		const indicator = page.getByRole('img', {
			name: 'Guardrails acted on this request: Blocked, Redacted, Error'
		});
		await expect.element(indicator).toBeInTheDocument();
		// Only the request a guardrail acted on is marked.
		await expect
			.poll(() => page.getByRole('img', { name: /Guardrails acted/ }).all())
			.toHaveLength(1);

		await indicator.click();
		await vi.waitFor(() => expect(details).toHaveBeenCalledWith('audit-blocked'));
		const section = page.getByRole('region', { name: 'Guardrails' });
		await expect.element(section).toBeVisible();
		await expect.element(section.getByText('Block PII')).toBeVisible();
		await expect.element(section.getByText('Mask secrets')).toBeVisible();

		await page.getByRole('cell', { name: 'clean-agent' }).click();
		await vi.waitFor(() => expect(details).toHaveBeenCalledWith('audit-clean'));
		await expect.element(section).not.toBeInTheDocument();
	});
});
