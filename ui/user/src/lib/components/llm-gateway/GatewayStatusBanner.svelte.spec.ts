import GatewayStatusBanner from './GatewayStatusBanner.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const readyStatus = {
	enabled: true,
	ready: true,
	hasProviders: true,
	modelProviderName: 'llm-gateway-model-provider',
	proxyPath: '/api/llm-proxy'
};

const startingText = 'The LLM gateway is starting…';
const noProvidersText = 'No upstream providers are configured yet.';

describe('GatewayStatusBanner', () => {
	it('renders nothing while the status is unknown', async () => {
		render(GatewayStatusBanner, {});

		await expect.element(page.getByText(startingText)).not.toBeInTheDocument();
		await expect.element(page.getByText(noProvidersText, { exact: false })).not.toBeInTheDocument();
	});

	it('renders nothing when the gateway is ready with providers', async () => {
		render(GatewayStatusBanner, { status: readyStatus });

		await expect.element(page.getByText(startingText)).not.toBeInTheDocument();
		await expect.element(page.getByText(noProvidersText, { exact: false })).not.toBeInTheDocument();
	});

	it('shows the startup error', async () => {
		render(GatewayStatusBanner, {
			status: { ...readyStatus, ready: false, error: 'listen tcp: address in use' }
		});

		await expect
			.element(page.getByText('The LLM gateway failed to start: listen tcp: address in use'))
			.toBeInTheDocument();
		await expect.element(page.getByText(startingText)).not.toBeInTheDocument();
	});

	it('shows that the gateway is starting when it is not ready yet', async () => {
		render(GatewayStatusBanner, { status: { ...readyStatus, ready: false } });

		await expect.element(page.getByText(startingText)).toBeInTheDocument();
	});

	it('links to provider setup when no providers are configured', async () => {
		render(GatewayStatusBanner, { status: { ...readyStatus, hasProviders: false } });

		await expect.element(page.getByText(noProvidersText, { exact: false })).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'Add a provider' }))
			.toHaveAttribute('href', '/models?view=providers');
	});

	it('can hide the no-providers notice', async () => {
		render(GatewayStatusBanner, {
			status: { ...readyStatus, hasProviders: false },
			showNoProviders: false
		});

		await expect.element(page.getByText(noProvidersText, { exact: false })).not.toBeInTheDocument();
	});
});
