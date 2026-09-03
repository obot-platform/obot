import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { errors, productTelemetryConsent, profile } from '$lib/stores';
import {
	clearProductAnalyticsPromptDismissal,
	isProductAnalyticsPromptDismissed
} from '$lib/stores/productTelemetryConsent.svelte';
import { createMockProfile } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ProductAnalyticsConsentDialog from './ProductAnalyticsConsentDialog.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function renderDialog(groups: string[] = [Group.ADMIN], consent?: boolean, available = true) {
	profile.initialize(createMockProfile(groups));
	productTelemetryConsent.initialize({ consent }, available);
	return render(ProductAnalyticsConsentDialog);
}

describe('ProductAnalyticsConsentDialog', () => {
	beforeEach(() => {
		errors.items = [];
	});

	it.each([
		['Admin', [Group.ADMIN]],
		['Owner', [Group.OWNER, Group.ADMIN]]
	])('prompts an undecided %s', async (_name, groups) => {
		await renderDialog(groups);
		await expect
			.element(page.getByRole('dialog').getByText('Help improve Obot', { exact: true }))
			.toBeVisible();
	});

	it.each([
		['auditor', [Group.AUDITOR]],
		['power user', [Group.POWERUSER]],
		['basic user', [Group.USER]]
	])('does not prompt a %s', async (_name, groups) => {
		await renderDialog(groups);
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
	});

	it.each([true, false])('does not prompt after consent is %s', async (consent) => {
		await renderDialog([Group.ADMIN], consent);
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
	});

	it('does not prompt when the consent API is unavailable', async () => {
		await renderDialog([Group.ADMIN], undefined, false);
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
	});

	it('does not prompt on the Product Analytics Platform tab', async () => {
		const originalUrl = Object.getOwnPropertyDescriptor(appPage, 'url');
		Object.defineProperty(appPage, 'url', {
			configurable: true,
			value: new URL('/admin/platform?view=product-analytics', window.location.origin)
		});

		try {
			await renderDialog();
			await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
		} finally {
			if (originalUrl) Object.defineProperty(appPage, 'url', originalUrl);
		}
	});

	it.each([
		['Share analytics', true],
		['Don’t share', false]
	])('persists %s explicitly', async (buttonName, consent) => {
		const update = vi.fn();
		worker.use(
			http.put('/api/product-telemetry-consent', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({ consent });
			})
		);

		await renderDialog();
		await page.getByRole('button', { name: buttonName, exact: true }).click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ consent });
			expect(productTelemetryConsent.consent).toBe(consent);
		});
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
	});

	it('dismisses without writing and stays suppressed until logout', async () => {
		const update = vi.fn();
		worker.use(http.put('/api/product-telemetry-consent', update));

		await renderDialog();
		await page.getByCSS('#product-analytics-consent-dialog .dialog-close-btn').click();

		expect(update).not.toHaveBeenCalled();
		expect(isProductAnalyticsPromptDismissed()).toBe(true);
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();

		await renderDialog();
		expect(
			[...document.querySelectorAll<HTMLDialogElement>('#product-analytics-consent-dialog')].every(
				(dialog) => !dialog.open
			)
		).toBe(true);

		clearProductAnalyticsPromptDismissal();
		await renderDialog();
		await vi.waitFor(() => {
			expect(
				[...document.querySelectorAll<HTMLDialogElement>('#product-analytics-consent-dialog')].some(
					(dialog) => dialog.open
				)
			).toBe(true);
		});
	});

	it('keeps the prompt open and permits retry after a failed write', async () => {
		let attempts = 0;
		worker.use(
			http.put('/api/product-telemetry-consent', () => {
				attempts++;
				return attempts === 1
					? HttpResponse.json({ error: 'try again' }, { status: 500 })
					: HttpResponse.json({ consent: true });
			})
		);

		await renderDialog();
		const share = page.getByRole('button', { name: 'Share analytics', exact: true });
		await share.click();

		await vi.waitFor(() => expect(errors.items).toHaveLength(1));
		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(share).toBeEnabled();

		await share.click();
		await vi.waitFor(() => expect(productTelemetryConsent.consent).toBe(true));
		await expect.element(page.getByCSS('#product-analytics-consent-dialog')).not.toBeVisible();
	});
});
