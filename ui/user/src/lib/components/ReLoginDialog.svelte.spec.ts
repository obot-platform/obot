import { profile } from '$lib/stores';
import {
	deferProductAnalyticsConsent,
	isProductAnalyticsConsentDeferred
} from '$lib/stores/productTelemetryConsent.svelte';
import { createMockProfile } from '../../tests/helpers/pageData';
import ReLoginDialog from './ReLoginDialog.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('clears deferred product analytics consent when the session expires', async () => {
	deferProductAnalyticsConsent();
	profile.initialize({ ...createMockProfile(), expired: true });

	await render(ReLoginDialog);

	await expect.element(page.getByRole('dialog')).toBeVisible();
	expect(isProductAnalyticsConsentDeferred()).toBe(false);
});
