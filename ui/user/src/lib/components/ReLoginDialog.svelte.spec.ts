import { profile } from '$lib/stores';
import {
	dismissProductAnalyticsPrompt,
	isProductAnalyticsPromptDismissed
} from '$lib/stores/productTelemetryConsent.svelte';
import { createMockProfile } from '../../tests/helpers/pageData';
import ReLoginDialog from './ReLoginDialog.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('clears product analytics prompt dismissal when the session expires', async () => {
	dismissProductAnalyticsPrompt();
	profile.initialize({ ...createMockProfile(), expired: true });

	await render(ReLoginDialog);

	await expect.element(page.getByRole('dialog')).toBeVisible();
	expect(isProductAnalyticsPromptDismissed()).toBe(false);
});
