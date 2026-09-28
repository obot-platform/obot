import { Group, type ModelProxySettings } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import ModelProxyView from './ModelProxyView.svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const proxyUrl = 'https://model-service.obot.ai';

function renderView(settings: ModelProxySettings = { enabled: false, url: proxyUrl }) {
	return render(ModelProxyView, { settings });
}

describe('Model proxy settings view', () => {
	beforeEach(() => {
		profile.initialize(createMockProfile([Group.ADMIN]));
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
