import { Group, type ModelProxySettings, type ModelProxyUsage } from '$lib/services';
import { profile, version } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { getVersionResponse } from '../../../tests/mocks/data';
import ModelProxyView from './ModelProxyView.svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const proxyUrl = 'https://model-service.obot.ai';

const usage: ModelProxyUsage = {
	input: { max: 1000, used: 250 },
	output: { max: 100, used: 50 },
	resetAt: '2099-01-01T00:00:00Z'
};

function renderView(
	settings: ModelProxySettings = { enabled: false, url: proxyUrl },
	usage?: ModelProxyUsage
) {
	return render(ModelProxyView, { settings, usage });
}

describe('Model proxy settings view', () => {
	beforeEach(() => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		version.initialize({ ...getVersionResponse, hasModelProvider: false });
	});

	it('shows usage when no model provider is configured', async () => {
		await renderView({ enabled: true, url: proxyUrl }, usage);

		await expect.element(page.getByText('75.0% remaining')).toBeInTheDocument();
		await expect.element(page.getByText(/model proxy isn't used/)).not.toBeInTheDocument();
	});

	it('explains that the model proxy is unused when a model provider is configured', async () => {
		version.initialize({ ...getVersionResponse, hasModelProvider: true });
		await renderView({ enabled: true, url: proxyUrl }, usage);

		await expect
			.element(
				page.getByText(
					"The model proxy isn't used because a model provider is configured. The vMCP Inspector uses the model provider instead."
				)
			)
			.toBeInTheDocument();
		await expect.element(page.getByText('Input tokens')).not.toBeInTheDocument();
	});

	it('locks the toggle for read-only administrators', async () => {
		profile.initialize(createMockProfile([Group.AUDITOR]));
		await renderView({ enabled: true, url: proxyUrl });

		await expect
			.element(page.getByRole('checkbox', { name: /^Enable Model Proxy/ }))
			.toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
	});
});
