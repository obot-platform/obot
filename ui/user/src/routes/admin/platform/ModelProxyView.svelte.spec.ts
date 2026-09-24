import { invalidate } from '$app/navigation';
import { Group, type ModelProxySettings } from '$lib/services';
import { profile } from '$lib/stores';
import { success } from '$lib/stores/success';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ModelProxyView from './ModelProxyView.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		invalidate: vi.fn(async () => {})
	};
});

const proxyUrl = 'https://model-service.obot.ai';

function renderView(settings: ModelProxySettings = { enabled: false, url: proxyUrl }) {
	return render(ModelProxyView, { settings });
}

describe('Model proxy settings view', () => {
	beforeEach(() => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.mocked(invalidate).mockClear();
	});

	it.each([
		[false, true],
		[true, false]
	] as const)('saves a change from %s to %s', async (initial, selected) => {
		const update = vi.fn();
		const successNotification = vi.spyOn(success, 'add');
		worker.use(
			http.put('/api/model-proxy', async ({ request }) => {
				const body = await request.json();
				update(body);
				return HttpResponse.json({ enabled: selected, url: proxyUrl });
			})
		);

		await renderView({ enabled: initial, url: proxyUrl });
		const toggle = page.getByRole('checkbox', { name: /^Enable Model Proxy/ });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await toggle.click();
		await expect.element(save).toBeEnabled();
		await save.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ enabled: selected });
			expect(invalidate).toHaveBeenCalledWith('model-proxy:usage');
			expect(successNotification).toHaveBeenCalledWith(
				'Model proxy settings updated successfully.'
			);
		});
		await expect.element(save).toBeEnabled();
		if (selected) {
			await expect.element(toggle).toBeChecked();
		} else {
			await expect.element(toggle).not.toBeChecked();
		}

		successNotification.mockRestore();
	});

	it('hides save and locks the toggle for read-only administrators', async () => {
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
