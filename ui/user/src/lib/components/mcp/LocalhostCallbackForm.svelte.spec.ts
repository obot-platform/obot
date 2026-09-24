import LocalhostCallbackForm from './LocalhostCallbackForm.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Localhost OAuth configuration', () => {
	it('shows the callback path only when enabled', async () => {
		await render(LocalhostCallbackForm, { config: {} });
		await expect.element(page.getByLabelText('Callback path')).not.toBeInTheDocument();
		await page.getByRole('checkbox', { name: 'Localhost OAuth callback' }).click();
		await expect.element(page.getByLabelText('Callback path')).toBeVisible();
		await page.getByLabelText('Callback path').fill('/custom/callback');
		await expect.element(page.getByLabelText('Callback path')).toHaveValue('/custom/callback');
		await page.getByRole('checkbox', { name: 'Localhost OAuth callback' }).click();
		await expect.element(page.getByLabelText('Callback path')).not.toBeInTheDocument();
	});
	it('preserves existing settings in read-only views', async () => {
		await render(LocalhostCallbackForm, {
			config: { localhostCallbackEnabled: true, localhostCallbackPath: '/custom' },
			readonly: true
		});
		await expect.element(page.getByRole('checkbox')).toBeChecked();
		await expect.element(page.getByRole('checkbox')).toBeDisabled();
		await expect.element(page.getByLabelText('Callback path')).toHaveValue('/custom');
		await expect.element(page.getByLabelText('Callback path')).toBeDisabled();
	});
});
