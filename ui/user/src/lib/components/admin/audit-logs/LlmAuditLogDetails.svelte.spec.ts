import { page as appPage } from '$app/state';
import type { LLMAuditLog } from '$lib/services';
import AuditLogsPage from '../../../../routes/audit-logs/+page.svelte';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const entry: LLMAuditLog = {
	id: 'audit-cached',
	createdAt: '2026-10-01T12:00:00Z',
	duration: 1250,
	userID: 'user-1',
	modelProvider: 'llm-gateway-model-provider',
	modelID: 'm1',
	targetModel: 'openai/gpt-4o-mini',
	servedProvider: 'openai',
	servedModel: 'gpt-4o-mini',
	reasoningEffort: '',
	requestPath: '/api/llm-proxy/v1/chat/completions',
	requestMethod: 'POST',
	requestID: 'req-cached',
	responseID: '',
	responseStatus: 200,
	outcome: 'success',
	inputTokens: 3132,
	cacheReadTokens: 3072,
	outputTokens: 40,
	reasoningTokens: 12,
	spend: 0.0023,
	userAgent: 'cached-agent',
	clientSessionID: '',
	clientIP: '10.0.0.1'
};

describe('LLM audit log usage', () => {
	afterEach(() => {
		appPage.url.searchParams.delete('view');
	});

	it("shows a request's spend, and the cached and reasoning tokens in the details", async () => {
		worker.use(
			http.get('/api/llm-audit-logs', () =>
				HttpResponse.json({ items: [entry], total: 1, limit: 100, offset: 0 })
			),
			http.get('/api/llm-audit-logs/detail/:id', () => HttpResponse.json(entry))
		);
		appPage.url.searchParams.set('view', 'llm');
		await preparePageData();
		render(AuditLogsPage);

		await expect.element(page.getByRole('columnheader', { name: 'Spend' })).toBeInTheDocument();
		await expect.element(page.getByRole('cell', { name: '$0.0023' })).toBeInTheDocument();

		await page.getByRole('cell', { name: 'cached-agent' }).click();
		await expect
			.element(page.getByText(/3,132 input \(3,072 read from cache\) · 40 output \(12 reasoning\)/))
			.toBeVisible();
		await expect.element(page.getByText(/Spend:\s*\$0\.0023/)).toBeVisible();
	});
});
