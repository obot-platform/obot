import { page as appPage } from '$app/state';
import type { LLMAuditLog } from '$lib/services';
import AuditLogsPage from '../../../../routes/audit-logs/+page.svelte';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import LlmAuditLogRouting, { servedBy, wasRouted } from './LlmAuditLogRouting.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function auditLog(overrides: Partial<LLMAuditLog>): LLMAuditLog {
	return {
		id: 'audit-1',
		createdAt: '2026-10-01T12:00:00Z',
		duration: 1250,
		userID: 'user-1',
		modelProvider: 'llm-gateway-model-provider',
		modelID: 'm1',
		targetModel: 'openai/gpt-4o',
		servedProvider: 'openai',
		servedModel: 'gpt-4o',
		providerKey: 'openai-primary',
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

describe('LlmAuditLogRouting', () => {
	it('tells when a request was served by another model', () => {
		expect(servedBy(auditLog({}))).toBe('openai/gpt-4o');
		expect(wasRouted(auditLog({}))).toBe(false);
		expect(
			wasRouted(auditLog({ servedProvider: 'anthropic', servedModel: 'claude-sonnet-4-5' }))
		).toBe(true);
		// Entries recorded before routing was recorded have nothing to compare.
		expect(wasRouted(auditLog({ servedProvider: undefined, servedModel: undefined }))).toBe(false);
	});

	it('lists the requested and served model, key, fallback and duration', async () => {
		render(LlmAuditLogRouting, {
			auditLog: auditLog({
				servedProvider: 'anthropic',
				servedModel: 'claude-sonnet-4-5',
				providerKey: 'anthropic-backup',
				fallback: true
			})
		});

		const section = page.getByRole('region', { name: 'Routing' });
		await expect.element(section).toBeVisible();
		await expect.element(section.getByText('openai/gpt-4o')).toBeVisible();
		await expect.element(section.getByText('anthropic/claude-sonnet-4-5')).toBeVisible();
		await expect.element(section.getByText('anthropic-backup')).toBeVisible();
		await expect.element(section.getByText('Yes', { exact: true })).toBeVisible();
		await expect.element(section.getByText(/1,250\s*ms/)).toBeVisible();
		await expect.element(section.getByText('Paid by')).not.toBeInTheDocument();
	});

	it('wraps a long key name between its words, not in the middle of one', async () => {
		const key = 'OPENAI_API_KEY_team_rotation_secondary';
		const view = await render(LlmAuditLogRouting, { auditLog: auditLog({ providerKey: key }) });
		view.container.style.width = '300px';

		const value = page.getByRole('region', { name: 'Routing' }).getByText(key);
		await expect.element(value).toBeVisible();
		// The characters each line ends with: a line may only end after a _, never mid-word.
		const lineEnds = () => {
			const dd = value.element() as HTMLElement;
			const walker = document.createTreeWalker(dd, NodeFilter.SHOW_TEXT);
			const chars: { char: string; top: number }[] = [];
			for (let node = walker.nextNode(); node; node = walker.nextNode()) {
				for (let i = 0; i < node.textContent!.length; i++) {
					const range = document.createRange();
					range.setStart(node, i);
					range.setEnd(node, i + 1);
					chars.push({ char: node.textContent![i], top: range.getBoundingClientRect().top });
				}
			}
			const text = chars
				.map((c) => c.char)
				.join('')
				.trim();
			const ends = chars
				.filter((c, i) => i + 1 < chars.length && chars[i + 1].top > c.top + 1)
				.map((c) => c.char);
			return { text, ends };
		};
		const { text, ends } = lineEnds();
		expect(text).toBe(key);
		expect(ends.length).toBeGreaterThan(0);
		expect(ends.every((c) => c === '_')).toBe(true);
	});

	it("says when the user's Claude subscription paid for a request", async () => {
		render(LlmAuditLogRouting, {
			auditLog: auditLog({
				targetModel: 'anthropic/claude-sonnet-4-5',
				servedProvider: 'anthropic',
				servedModel: 'claude-sonnet-4-5',
				providerKey: undefined,
				claudeSubscription: true
			})
		});

		const section = page.getByRole('region', { name: 'Routing' });
		await expect.element(section.getByText('Paid by')).toBeVisible();
		await expect.element(section.getByText("The user's Claude subscription")).toBeVisible();
		await expect.element(section.getByText('Provider key')).not.toBeInTheDocument();
	});
});

describe('Model audit logs routing', () => {
	afterEach(() => {
		appPage.url.searchParams.delete('view');
	});

	it('marks routed requests and shows the routing in the details', async () => {
		const routed = auditLog({
			id: 'audit-routed',
			requestID: 'req-routed',
			servedProvider: 'anthropic',
			servedModel: 'claude-sonnet-4-5',
			providerKey: 'anthropic-backup',
			fallback: true,
			userAgent: 'routed-agent'
		});
		const direct = auditLog({
			id: 'audit-direct',
			requestID: 'req-direct',
			userAgent: 'direct-agent'
		});
		const details = vi.fn();
		worker.use(
			http.get('/api/llm-audit-logs', () =>
				HttpResponse.json({ items: [routed, direct], total: 2, limit: 100, offset: 0 })
			),
			http.get('/api/llm-audit-logs/detail/:id', ({ params }) => {
				details(params.id);
				return HttpResponse.json(params.id === routed.id ? routed : direct);
			})
		);
		appPage.url.searchParams.set('view', 'llm');
		await preparePageData();
		render(AuditLogsPage);

		const marker = page.getByRole('img', {
			name: 'Requested openai/gpt-4o, served by anthropic/claude-sonnet-4-5'
		});
		await expect.element(marker).toBeInTheDocument();
		// Only the routed request is marked.
		await expect.poll(() => page.getByRole('img', { name: /served by/ }).all()).toHaveLength(1);
		await expect.element(page.getByRole('cell', { name: 'anthropic', exact: true })).toBeVisible();

		await page.getByRole('cell', { name: 'routed-agent' }).click();
		await vi.waitFor(() => expect(details).toHaveBeenCalledWith('audit-routed'));
		const section = page.getByRole('region', { name: 'Routing' });
		await expect.element(section.getByText('anthropic-backup')).toBeVisible();
	});
});
