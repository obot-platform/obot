import { page as appPage } from '$app/state';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import LlmAuditLogsContent from './LlmAuditLogsContent.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function mockLlmAuditLogApis() {
	const requests = {
		auditLogs: undefined as string | undefined
	};

	worker.use(
		http.get('/api/llm-audit-logs', ({ request }) => {
			requests.auditLogs = request.url;
			return HttpResponse.json({ items: [], total: 0 });
		})
	);

	return requests;
}

async function renderLlmAuditLogs() {
	const requests = mockLlmAuditLogApis();
	await preparePageData();
	render(LlmAuditLogsContent);
	return requests;
}

async function auditLogParams(requests: { auditLogs: string | undefined }) {
	await vi.waitFor(() => expect(requests.auditLogs).toBeTruthy());
	return new URL(requests.auditLogs!).searchParams;
}

afterEach(() => {
	appPage.url.searchParams.delete('hide_models_requests');
	vi.restoreAllMocks();
});

describe('LlmAuditLogsContent default filters', () => {
	it('applies the default model-discovery filter on first load', async () => {
		const requests = await renderLlmAuditLogs();

		expect((await auditLogParams(requests)).get('hide_models_requests')).toBe('true');
		await expect
			.element(page.getByCSS('.filter-primary').filter({ hasText: 'Model discovery requests' }))
			.toBeVisible();
	});

	it('does not restore the default model-discovery filter when the URL param is empty', async () => {
		appPage.url.searchParams.set('hide_models_requests', '');

		const requests = await renderLlmAuditLogs();

		expect((await auditLogParams(requests)).get('hide_models_requests')).toBeNull();
		await expect
			.element(page.getByCSS('.filter-primary').filter({ hasText: 'Model discovery requests' }))
			.not.toBeInTheDocument();
	});
});
