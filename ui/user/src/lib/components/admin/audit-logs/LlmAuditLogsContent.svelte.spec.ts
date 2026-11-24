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
	(appPage.url as URL).searchParams.delete('include_on_behalf_of');
	(appPage.url as URL).searchParams.delete('hide_models_requests');
	(appPage.url as URL).searchParams.delete('user_id');
	(appPage.url as URL).searchParams.delete('agent_id');
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
		(appPage.url as URL).searchParams.set('hide_models_requests', '');

		const requests = await renderLlmAuditLogs();

		expect((await auditLogParams(requests)).get('hide_models_requests')).toBeNull();
		await expect
			.element(page.getByCSS('.filter-primary').filter({ hasText: 'Model discovery requests' }))
			.not.toBeInTheDocument();
	});
});

// An agent is filtered for as the owner it is, "agent:<id>" in user_id. There is no agent_id filter.
describe('LlmAuditLogsContent agent filter', () => {
	it('filters for an agent by user_id and names it, and sends no agent_id', async () => {
		worker.use(
			http.get('/api/agents', () =>
				HttpResponse.json({
					items: [{ id: '7', displayName: 'Build Bot', status: 'active', activeAPIKeyCount: 1 }]
				})
			)
		);
		(appPage.url as URL).searchParams.set('user_id', 'agent:7');
		(appPage.url as URL).searchParams.set('agent_id', '7');

		const params = await auditLogParams(await renderLlmAuditLogs());

		expect(params.get('user_id')).toBe('agent:7');
		expect(params.get('agent_id')).toBeNull();
		const pill = page.getByCSS('.filter-primary').filter({ hasText: /Build Bot|Agent 7/ });
		await expect.element(pill).toBeVisible();
		await expect.element(pill.getByText('agent:7')).not.toBeInTheDocument();
	});
});

describe('LlmAuditLogsContent on behalf of a user', () => {
	const toggle = () =>
		page.getByRole('checkbox', { name: 'Include actions on behalf of this user' });

	it('asks for the requests made on behalf of the user when the toggle is on', async () => {
		(appPage.url as URL).searchParams.set('user_id', '3');
		(appPage.url as URL).searchParams.set('include_on_behalf_of', 'true');

		const requests = await renderLlmAuditLogs();

		await expect.element(toggle()).toBeChecked();
		expect((await auditLogParams(requests)).get('include_on_behalf_of')).toBe('true');
	});

	it('leaves the parameter out when the toggle is off', async () => {
		(appPage.url as URL).searchParams.set('user_id', '3');

		const requests = await renderLlmAuditLogs();

		await expect.element(toggle()).not.toBeChecked();
		expect((await auditLogParams(requests)).has('include_on_behalf_of')).toBe(false);
	});

	it('offers no toggle without a filter on a user, or with one on an agent', async () => {
		(appPage.url as URL).searchParams.set('user_id', 'agent:7');
		(appPage.url as URL).searchParams.set('include_on_behalf_of', 'true');

		const requests = await renderLlmAuditLogs();

		await expect.element(toggle()).not.toBeInTheDocument();
		// An agent has no one to act for, so the parameter is not sent either.
		expect((await auditLogParams(requests)).has('include_on_behalf_of')).toBe(false);
	});
});
