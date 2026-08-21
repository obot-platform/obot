import { page as appPage } from '$app/state';
import { ApiKeysService, Group } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile, preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import type { PageData } from './$types';
import { load } from './+page';
import AgentAuthScopeApiKeyPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const apiKey: APIKey = {
	id: 42,
	userId: 7,
	name: 'Test Agent Scope',
	canAccessAPI: false,
	canAccessLLMProxy: true,
	canAccessSkills: false,
	canAccessDeviceScans: false,
	createdAt: '2026-01-01T00:00:00.000Z'
};

const apiKeyPrefix = `ok1-${apiKey.userId}-${apiKey.id}-*****`;

function loadAgentAuthScopeApiKey(pathname: string) {
	const profile = createMockProfile();
	return load({
		fetch: vi.fn(),
		params: { id: apiKey.id.toString(), api_key: apiKeyPrefix },
		parent: vi.fn(async () => ({ profile })),
		url: new URL(pathname, 'http://localhost')
	} as unknown as Parameters<typeof load>[0]);
}

function mockUsageAndLogApis() {
	const requests = {
		tokenUsage: undefined as string | undefined,
		mcpAuditLogs: undefined as string | undefined
	};

	worker.use(
		http.get('/api/users/:userId/token-usage', ({ request }) => {
			requests.tokenUsage = request.url;
			return HttpResponse.json({
				items: [
					{
						date: '2026-08-17T00:00:00.000Z',
						userID: String(apiKey.userId),
						apiKeyID: apiKey.id,
						apiKeyName: apiKey.name,
						inputTokens: 100,
						cacheReadTokens: 10,
						cacheWriteTokens: 5,
						outputTokens: 50,
						thinkingTokens: 0,
						totalTokens: 150,
						inputSpend: 1,
						cacheReadSpend: 0.1,
						cacheWriteSpend: 0.05,
						outputSpend: 0.5,
						totalSpend: 1.5
					},
					{
						date: '2026-08-17T00:00:00.000Z',
						userID: String(apiKey.userId),
						apiKeyID: 99,
						inputTokens: 999,
						cacheReadTokens: 0,
						cacheWriteTokens: 0,
						outputTokens: 999,
						thinkingTokens: 0,
						totalTokens: 1998,
						inputSpend: 9,
						cacheReadSpend: 0,
						cacheWriteSpend: 0,
						outputSpend: 9,
						totalSpend: 18
					}
				]
			});
		}),
		http.get('/api/token-usage', () => HttpResponse.json({ items: [] })),
		http.get('/api/mcp-audit-logs', ({ request }) => {
			requests.mcpAuditLogs = request.url;
			return HttpResponse.json({ items: [], total: 0 });
		}),
		http.get('/api/mcp-audit-logs/filter-options/:filter', () =>
			HttpResponse.json({ options: [] })
		),
		http.get('/api/llm-audit-logs', () => HttpResponse.json({ items: [], total: 0 })),
		http.get('/api/llm-audit-logs/filter-options/:filter', () => HttpResponse.json({ options: [] }))
	);

	return requests;
}

async function renderApiKeyPage({
	groups = [Group.ADMIN],
	isAdmin = true
}: {
	groups?: string[];
	isAdmin?: boolean;
} = {}) {
	const requests = mockUsageAndLogApis();
	const data = await preparePageData<PageData>({
		apiKey,
		apiKeyId: apiKeyPrefix,
		isAdmin,
		profile: createMockProfile(groups)
	});
	const rendered = render(AgentAuthScopeApiKeyPage, { data });
	return { rendered, requests };
}

afterEach(() => {
	appPage.url.searchParams.delete('start');
	appPage.url.searchParams.delete('end');
	vi.restoreAllMocks();
});

describe('Agent Auth Scope API key route selection', () => {
	it('gets current-user scope for non-admin URL', async () => {
		const getApiKey = vi.spyOn(ApiKeysService, 'getApiKey').mockResolvedValue(apiKey);
		const getAnyApiKey = vi.spyOn(ApiKeysService, 'getAnyApiKey');

		const result = await loadAgentAuthScopeApiKey(
			`/agent-auth-scopes/${apiKey.id}/${encodeURIComponent(apiKeyPrefix)}`
		);

		expect(result).toEqual({ apiKey, apiKeyId: apiKeyPrefix, isAdmin: false });
		expect(getApiKey).toHaveBeenCalledWith(apiKey.id.toString(), expect.any(Object));
		expect(getAnyApiKey).not.toHaveBeenCalled();
	});

	it('gets any scope for admin URL', async () => {
		const getApiKey = vi.spyOn(ApiKeysService, 'getApiKey');
		const getAnyApiKey = vi.spyOn(ApiKeysService, 'getAnyApiKey').mockResolvedValue(apiKey);

		const result = await loadAgentAuthScopeApiKey(
			`/admin/agent-auth-scopes/${apiKey.id}/${encodeURIComponent(apiKeyPrefix)}`
		);

		expect(result).toEqual({ apiKey, apiKeyId: apiKeyPrefix, isAdmin: true });
		expect(getApiKey).not.toHaveBeenCalled();
		expect(getAnyApiKey).toHaveBeenCalledWith(apiKey.id.toString(), expect.any(Object));
	});
});

describe('Agent Auth Scope API key usage page', () => {
	it('shows token usage totals for this API key and MCP log tab', async () => {
		const { requests } = await renderApiKeyPage();

		await expect.element(page.getByText('$1.50', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'MCP Server Logs' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'LLM Gateway Logs' })).toBeVisible();
		await expect.element(page.getByText('No MCP server logs', { exact: true })).toBeVisible();

		await vi.waitFor(() => {
			expect(requests.tokenUsage).toBeTruthy();
			expect(requests.mcpAuditLogs).toBeTruthy();
		});
		const tokenUsageParams = new URL(requests.tokenUsage!).searchParams;
		const mcpLogParams = new URL(requests.mcpAuditLogs!).searchParams;
		expect(mcpLogParams.get('start_time')).toBe(tokenUsageParams.get('start'));
		expect(mcpLogParams.get('end_time')).toBe(tokenUsageParams.get('end'));
		expect(mcpLogParams.get('api_key_id')).toBe(String(apiKey.id));
	});

	it('hides LLM gateway logs for users without admin access', async () => {
		await renderApiKeyPage({ groups: [Group.USER], isAdmin: false });

		await expect.element(page.getByRole('button', { name: 'MCP Server Logs' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'LLM Gateway Logs' }))
			.not.toBeInTheDocument();
	});

	it('falls back to the default range when start/end URL params are invalid', async () => {
		appPage.url.searchParams.set('start', 'not-a-date');
		appPage.url.searchParams.set('end', 'also-invalid');

		const { requests } = await renderApiKeyPage();

		await expect.element(page.getByText('$1.50', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByText('No MCP server logs', { exact: true })).toBeVisible();

		await vi.waitFor(() => {
			expect(requests.tokenUsage).toBeTruthy();
			expect(requests.mcpAuditLogs).toBeTruthy();
		});

		const tokenUsageParams = new URL(requests.tokenUsage!).searchParams;
		const mcpLogParams = new URL(requests.mcpAuditLogs!).searchParams;
		expect(() => new Date(tokenUsageParams.get('start')!).toISOString()).not.toThrow();
		expect(() => new Date(tokenUsageParams.get('end')!).toISOString()).not.toThrow();
		expect(Number.isNaN(new Date(tokenUsageParams.get('start')!).getTime())).toBe(false);
		expect(Number.isNaN(new Date(tokenUsageParams.get('end')!).getTime())).toBe(false);
		expect(mcpLogParams.get('start_time')).toBe(tokenUsageParams.get('start'));
		expect(mcpLogParams.get('end_time')).toBe(tokenUsageParams.get('end'));
	});
});
