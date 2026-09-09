import { page as appPage } from '$app/state';
import { AdminService, Group } from '$lib/services';
import { productTelemetryConsent, profile, version } from '$lib/stores';
import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
import { isProductAnalyticsConsentDeferred } from '$lib/stores/productTelemetryConsent.svelte';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import SetupSplashDialog from './SetupSplashDialog.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const originalUrl = Object.getOwnPropertyDescriptor(appPage, 'url');

function setUrl(path = '/') {
	Object.defineProperty(appPage, 'url', {
		configurable: true,
		value: new URL(path, window.location.origin)
	});
}

async function configureCompletedWelcome() {
	worker.use(
		http.get('/api/auth-providers', () => HttpResponse.json({ items: [{ configured: true }] })),
		http.get('/api/model-providers', () => HttpResponse.json({ items: [{ configured: true }] })),
		http.get('/api/eula', () => HttpResponse.json({ accepted: true }))
	);
	await adminConfigStore.fetchData(true);
}

async function renderWelcome(
	groups: string[] = [Group.ADMIN],
	consent?: boolean,
	available = true
) {
	profile.initialize(createMockProfile(groups));
	version.initialize(getVersionResponse);
	productTelemetryConsent.initialize({ consent }, available);
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await configureCompletedWelcome();
	return render(SetupSplashDialog);
}

describe('SetupSplashDialog product analytics consent', () => {
	beforeEach(() => {
		setUrl();
	});

	afterEach(() => {
		if (originalUrl) Object.defineProperty(appPage, 'url', originalUrl);
		adminConfigStore.updateAuthProviders([
			{ configured: true } as Awaited<ReturnType<typeof AdminService.listAuthProviders>>[number]
		]);
		adminConfigStore.updateModelProviders([
			{ configured: true } as Awaited<ReturnType<typeof AdminService.listModelProviders>>[number]
		]);
		adminConfigStore.updateEula(true);
	});

	it.each([
		['Admin', [Group.ADMIN]],
		['Owner', [Group.OWNER, Group.ADMIN]]
	])(
		'opens the Welcome dialog for an undecided %s after setup is complete',
		async (_name, groups) => {
			await renderWelcome(groups);

			await expect.element(page.getByRole('dialog')).toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Welcome to Obot!', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('checkbox', { name: /Share product usage data/ }))
				.toBeChecked();
		}
	);

	it('uses the concise copy and links to the Product Analytics documentation', async () => {
		await renderWelcome();

		await expect
			.element(page.getByText('Share product usage data to help improve Obot.', { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Learn more', exact: true }))
			.toHaveAttribute('href', 'https://docs.obot.ai/configuration/product-analytics');
	});

	it.each([
		[true, true],
		[false, false]
	])('persists %s when the checkbox selected state is %s', async (selected, expected) => {
		const update = vi.fn();
		worker.use(
			http.put('/api/product-telemetry-consent', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({ consent: expected });
			})
		);

		await renderWelcome();
		const checkbox = page.getByRole('checkbox', { name: /Share product usage data/ });
		if (!selected) await checkbox.click();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ consent: expected });
			expect(productTelemetryConsent.consent).toBe(expected);
		});
		await expect.element(page.getByCSS('dialog')).not.toBeVisible();
	});

	it('continues undecided and defers another prompt when saving fails', async () => {
		worker.use(
			http.put('/api/product-telemetry-consent', () =>
				HttpResponse.json({ error: 'try again' }, { status: 500 })
			)
		);

		await renderWelcome();
		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await vi.waitFor(() => expect(isProductAnalyticsConsentDeferred()).toBe(true));
		expect(productTelemetryConsent.consent).toBeUndefined();
		await expect.element(page.getByCSS('dialog')).not.toBeVisible();
	});

	it.each([
		['a basic user', [Group.USER], undefined, true],
		['recorded consent', [Group.ADMIN], true, true],
		['unavailable consent controls', [Group.ADMIN], undefined, false]
	] as const)('does not open solely for %s', async (_name, groups, consent, available) => {
		await renderWelcome([...groups], consent, available);
		await expect.element(page.getByCSS('dialog')).not.toBeVisible();
	});

	it('does not open solely for consent on the Product Analytics settings tab', async () => {
		setUrl('/admin/platform?view=product-analytics');
		await renderWelcome();
		await expect.element(page.getByCSS('dialog')).not.toBeVisible();
	});

	it('preserves the existing setup trigger when consent is already recorded', async () => {
		worker.use(
			http.get('/api/auth-providers', () => HttpResponse.json({ items: [] })),
			http.get('/api/model-providers', () => HttpResponse.json({ items: [] })),
			http.get('/api/eula', () => HttpResponse.json({ accepted: false }))
		);
		profile.initialize(createMockProfile([Group.OWNER, Group.ADMIN]));
		version.initialize(getVersionResponse);
		productTelemetryConsent.initialize({ consent: false }, true);
		await adminConfigStore.fetchData(true);

		await render(SetupSplashDialog);

		await expect.element(page.getByRole('dialog')).toBeVisible();
		expect(document.querySelector('#share-product-usage')).toBeNull();
	});
});
