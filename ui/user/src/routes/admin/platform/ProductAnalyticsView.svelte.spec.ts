import type { ProductTelemetryConsent } from '$lib/services';
import { productTelemetryConsent } from '$lib/stores';
import ProductAnalyticsView from './ProductAnalyticsView.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function renderView(consent?: boolean, storeConsent = consent) {
	const currentConsent = { consent } satisfies ProductTelemetryConsent;
	productTelemetryConsent.initialize({ consent: storeConsent }, true);
	return render(ProductAnalyticsView, { consent: currentConsent });
}

describe('Product Analytics settings view', () => {
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

	it('disables both choices while the shared form is saving', async () => {
		productTelemetryConsent.initialize({ consent: false }, true);
		render(ProductAnalyticsView, { consent: { consent: false }, saving: true });

		await expect
			.element(page.getByRole('radio', { name: 'Enable product analytics', exact: true }))
			.toBeDisabled();
		await expect
			.element(page.getByRole('radio', { name: 'Disable product analytics', exact: true }))
			.toBeDisabled();
	});
});
