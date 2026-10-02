import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import AuditLogsPageContent from './AuditLogsPageContent.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	return {
		...actual,
		goto: vi.fn()
	};
});

function mockAuditLogApis(filterOptions: Record<string, string[]> = {}) {
	const requests = {
		auditLogs: undefined as string | undefined,
		filterOptions: [] as string[]
	};

	worker.use(
		http.get('/api/mcp-audit-logs', ({ request }) => {
			requests.auditLogs = request.url;
			return HttpResponse.json({ items: [], total: 0 });
		}),
		http.get('/api/mcp-audit-logs/filter-options/:filter', ({ request, params }) => {
			requests.filterOptions.push(request.url);
			return HttpResponse.json({ options: filterOptions[String(params.filter)] ?? [] });
		})
	);

	return requests;
}

async function renderAuditLogs(
	props: Record<string, unknown> = {},
	filterOptions: Record<string, string[]> = {}
) {
	const requests = mockAuditLogApis(filterOptions);
	await preparePageData();
	render(AuditLogsPageContent, props);
	return requests;
}

async function auditLogParams(requests: { auditLogs: string | undefined }) {
	await vi.waitFor(() => expect(requests.auditLogs).toBeTruthy());
	return new URL(requests.auditLogs!).searchParams;
}

async function filterOptionsParams(requests: { filterOptions: string[] }) {
	await vi.waitFor(() => expect(requests.filterOptions.length).toBeGreaterThan(0));
	return requests.filterOptions.map((url) => new URL(url).searchParams);
}

afterEach(() => {
	appPage.url.searchParams.delete('mcp_id');
	appPage.url.searchParams.delete('mcp_server_display_name');
	appPage.url.searchParams.delete('mcp_server');
	appPage.url.searchParams.delete('operation');
	appPage.url.searchParams.delete('client');
	vi.mocked(goto).mockClear();
	vi.restoreAllMocks();
});

describe('AuditLogsPageContent server scoping', () => {
	it('preserves comma-containing server names in requests and filter pills', async () => {
		const name = 'Outlook, Calendar';
		const value = JSON.stringify([name]);
		appPage.url.searchParams.set('mcp_server', value);

		const requests = await renderAuditLogs();
		expect((await auditLogParams(requests)).get('mcp_server')).toBe(value);
		await expect.element(page.getByText(name, { exact: true })).toBeVisible();
		const serverPill = page
			.getByCSS('.filter-primary')
			.filter({ hasText: 'Identifier – MCP Server' });
		await expect.element(serverPill.getByText('OR', { exact: true })).not.toBeInTheDocument();
	});

	it('applies mcp_id from the URL and pins the source to MCP', async () => {
		appPage.url.searchParams.set('mcp_id', 'server-1');

		const requests = await renderAuditLogs();

		const params = await auditLogParams(requests);
		expect(params.get('mcp_id')).toBe('server-1');
		expect(params.get('event_type')).toBe('mcp_call');
		await expect.element(page.getByText('Server ID', { exact: false }).first()).toBeVisible();
	});

	it('applies mcp_server_display_name from the URL and pins the source to MCP', async () => {
		appPage.url.searchParams.set('mcp_server_display_name', 'My Server');

		const requests = await renderAuditLogs();

		const params = await auditLogParams(requests);
		expect(params.get('mcp_server_display_name')).toBe('My Server');
		expect(params.get('event_type')).toBe('mcp_call');
	});

	it('pins the source to MCP when loading filter options for a URL-scoped server', async () => {
		appPage.url.searchParams.set('mcp_id', 'server-1');

		const requests = await renderAuditLogs();
		await page.getByRole('button', { name: 'Filters' }).click();

		const requestParams = await filterOptionsParams(requests);
		for (const params of requestParams) {
			expect(params.get('event_type')).toBe('mcp_call');
			expect(params.get('mcp_id')).toBe('server-1');
		}
	});

	it('still applies mcp_id passed as a prop', async () => {
		const requests = await renderAuditLogs({ mcpId: 'prop-server' });

		expect((await auditLogParams(requests)).get('mcp_id')).toBe('prop-server');
	});
});

describe('AuditLogsPageContent unified client filters', () => {
	it('keeps colliding raw client IDs visible and submits the selected ID', async () => {
		await renderAuditLogs({}, { client: ['claude-code', 'claude_code'] });
		await page.getByRole('button', { name: 'Filters' }).click();
		await page.getByCSS('#filter-client').click();

		await expect
			.element(page.getByText('claude-code · Claude Code', { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText('claude_code · Claude Code', { exact: true }))
			.toBeVisible();

		await page.getByText('claude_code · Claude Code', { exact: true }).click();
		await page.getByRole('button', { name: 'Apply Filters' }).click();

		await vi.waitFor(() => expect(vi.mocked(goto)).toHaveBeenCalled());
		const target = vi.mocked(goto).mock.calls.at(-1)?.[0] as URL;
		expect(target.searchParams.get('client')).toBe('claude_code');
	});
});

describe('AuditLogsPageContent default filters', () => {
	it('applies the default operation filter on first load', async () => {
		const requests = await renderAuditLogs();

		expect((await auditLogParams(requests)).get('operation')).toBe(
			'tools/call,resources/read,prompts/get'
		);
		await expect
			.element(page.getByCSS('.filter-primary').filter({ hasText: 'Operation' }))
			.toBeVisible();
	});

	it('does not restore the default operation filter when the URL param is empty', async () => {
		appPage.url.searchParams.set('operation', '');

		const requests = await renderAuditLogs();

		expect((await auditLogParams(requests)).get('operation')).toBeNull();
		await expect
			.element(page.getByCSS('.filter-primary').filter({ hasText: 'Operation' }))
			.not.toBeInTheDocument();
	});
});
