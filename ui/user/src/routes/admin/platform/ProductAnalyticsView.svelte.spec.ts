import type { ProductTelemetryConsent } from '$lib/services';
import { productTelemetryConsent } from '$lib/stores';
import { success } from '$lib/stores/success';
import { worker } from '../../../tests/mocks/worker';
import ProductAnalyticsView from './ProductAnalyticsView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function renderView(consent?: boolean, storeConsent = consent) {
	const currentConsent = { consent } satisfies ProductTelemetryConsent;
	productTelemetryConsent.initialize({ consent: storeConsent }, true);
	return render(ProductAnalyticsView, { consent: currentConsent });
}

describe('Product Analytics settings view', () => {
	it('uses the consent copy and links to additional details', async () => {
		await renderView();
		await expect
			.element(page.getByText(/Share product usage data to help improve Obot\./))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Learn more', exact: true }))
			.toHaveAttribute('href', 'https://docs.obot.ai/configuration/product-analytics');
		await expect
			.element(
				page.getByText(
					/software update checks are separate and may send the installation ID and current version/i
				)
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Learn more about update checks', exact: true }))
			.toHaveAttribute(
				'href',
				'https://docs.obot.ai/configuration/product-analytics#upgrade-checks-are-separate'
			);
	});

	it('prefers fresh route data over stale shared state and synchronizes the store', async () => {
		await renderView(true, false);
		await expect
			.element(page.getByRole('radio', { name: 'Enable product analytics', exact: true }))
			.toBeChecked();
		expect(productTelemetryConsent.consent).toBe(true);
	});

	it.each([
		[true, 'Enable product analytics'],
		[false, 'Disable product analytics']
	] as const)('renders consent %s as the selected radio', async (consent, radioName) => {
		await renderView(consent);
		await expect.element(page.getByRole('radio', { name: radioName, exact: true })).toBeChecked();
	});

	it('leaves both radios unselected when no decision is recorded', async () => {
		await renderView();
		await expect
			.element(page.getByRole('radio', { name: 'Enable product analytics', exact: true }))
			.not.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Disable product analytics', exact: true }))
			.not.toBeChecked();
	});

	it.each([
		[false, true],
		[true, false]
	] as const)('saves a change from %s to %s', async (initial, selected) => {
		const update = vi.fn();
		const successNotification = vi.spyOn(success, 'add');
		worker.use(
			http.put('/api/product-telemetry-consent', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({ consent: selected });
			})
		);

		await renderView(initial);
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await expect.element(save).toBeDisabled();
		await page
			.getByRole('radio', {
				name: selected ? 'Enable product analytics' : 'Disable product analytics',
				exact: true
			})
			.click();
		await expect.element(save).toBeEnabled();
		await save.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ consent: selected });
			expect(productTelemetryConsent.consent).toBe(selected);
			expect(successNotification).toHaveBeenCalledWith(
				'Product analytics preference updated successfully.'
			);
		});
		await expect
			.element(
				page.getByRole('radio', {
					name: selected ? 'Enable product analytics' : 'Disable product analytics',
					exact: true
				})
			)
			.toBeChecked();
		await expect.element(save).toBeDisabled();

		successNotification.mockRestore();
	});

	it('retains the unsaved choice when saving fails', async () => {
		worker.use(
			http.put('/api/product-telemetry-consent', () =>
				HttpResponse.json({ error: 'try again' }, { status: 500 })
			)
		);

		await renderView(false);
		const enabled = page.getByRole('radio', {
			name: 'Enable product analytics',
			exact: true
		});
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await enabled.click();
		await save.click();

		await expect.element(enabled).toBeChecked();
		await expect.element(save).toBeEnabled();
	});
});
