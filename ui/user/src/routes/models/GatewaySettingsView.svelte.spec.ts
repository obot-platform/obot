import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import SettingsPage from './GatewaySettingsView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const readyStatus = {
	enabled: true,
	ready: true,
	hasProviders: true,
	modelProviderName: 'llm-gateway-model-provider',
	proxyPath: '/api/llm-proxy'
};

const clientConfig = {
	drop_excess_requests: false,
	enable_logging: true,
	disable_content_logging: false,
	log_retention_days: 30,
	max_request_body_size_mb: 100,
	prometheus_labels: null,
	// Fields the page does not edit must be sent back unchanged.
	initial_pool_size: 300
};

const frameworkConfig = { pricing_sync_interval: 86400 };

function mockGateway({ status = readyStatus }: { status?: typeof readyStatus } = {}) {
	worker.use(
		http.get('/api/llm-gateway/status', () => HttpResponse.json(status)),
		http.get('/api/llm-gateway/admin/config', () =>
			HttpResponse.json({
				client_config: clientConfig,
				framework_config: frameworkConfig,
				is_logs_connected: true
			})
		),
		http.put('/api/llm-gateway/admin/config', () => HttpResponse.json({}))
	);
}

async function renderSettingsPage(groups: string[] = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	return render(SettingsPage);
}

function save() {
	return page.getByRole('button', { name: 'Save', exact: true });
}

describe('Models gateway settings tab', () => {
	it('loads the current traffic settings', async () => {
		mockGateway();
		await renderSettingsPage();

		await expect.element(page.getByLabelText('Max request body (MB)')).toHaveValue(100);
		await expect
			.element(page.getByRole('switch', { name: 'Reject requests when a provider is at capacity' }))
			.not.toBeChecked();
	});

	it("doesn't offer the gateway's own request log, which Obot doesn't use", async () => {
		mockGateway();
		await renderSettingsPage();

		await expect.element(page.getByRole('heading', { name: 'Traffic' })).toBeInTheDocument();
		for (const name of ['Log requests', 'Store request and response content']) {
			await expect.element(page.getByRole('switch', { name })).not.toBeInTheDocument();
		}
		await expect.element(page.getByLabelText('Log retention (days)')).not.toBeInTheDocument();
	});

	it('sends only the client config when saving', async () => {
		mockGateway();
		const saved = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/config', async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json({});
			})
		);
		await renderSettingsPage();

		await page
			.getByRole('switch', { name: 'Reject requests when a provider is at capacity' })
			.click();
		await page.getByLabelText('Max request body (MB)').fill('20');
		await save().click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		await expect.element(save()).toBeEnabled();
		expect(saved).toHaveBeenCalledWith({
			client_config: {
				...clientConfig,
				drop_excess_requests: true,
				max_request_body_size_mb: 20
			}
		});
	});

	it('leaves Claude subscriptions to the Anthropic provider', async () => {
		mockGateway();
		await renderSettingsPage();

		await expect.element(page.getByLabelText('Max request body (MB)')).toBeInTheDocument();
		await expect
			.element(page.getByRole('switch', { name: 'Allow Claude subscriptions' }))
			.not.toBeInTheDocument();
	});

	it('shows settings read-only to auditors', async () => {
		mockGateway();
		await renderSettingsPage([Group.USER, Group.AUDITOR]);

		await expect.element(page.getByLabelText('Max request body (MB)')).toBeDisabled();
		await expect
			.element(page.getByRole('switch', { name: 'Reject requests when a provider is at capacity' }))
			.toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
	});
});
