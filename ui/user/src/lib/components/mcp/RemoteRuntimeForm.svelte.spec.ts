import RemoteRuntimeForm from './RemoteRuntimeForm.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

for (const variant of ['catalog', 'server'] as const) {
	it(`places localhost OAuth under advanced configuration for ${variant}`, async () => {
		await render(RemoteRuntimeForm, {
			variant,
			config:
				variant === 'catalog'
					? { fixedURL: 'https://example.com/mcp' }
					: { url: 'https://example.com/mcp' }
		});
		const toggle = page.getByRole('switch', { name: /Localhost OAuth callback/ });
		await expect.element(toggle).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Advanced Configuration', exact: true }).click();
		await expect.element(toggle).toBeVisible();
		const path = page.getByLabelText('Callback path');
		await expect.element(path).not.toBeInTheDocument();
		await toggle.click();
		await expect.element(path).toBeEnabled();
		await path.fill('/custom/callback');
		await toggle.click();
		await expect.element(path).not.toBeInTheDocument();
		await toggle.click();
		await expect.element(path).toHaveValue('/custom/callback');
	});
}

it('opens advanced configuration for an existing localhost OAuth setting', async () => {
	await render(RemoteRuntimeForm, {
		config: {
			fixedURL: 'https://example.com/mcp',
			localhostCallbackEnabled: true,
			localhostCallbackPath: '/custom'
		}
	});
	await expect
		.element(page.getByRole('switch', { name: /Localhost OAuth callback/ }))
		.toBeChecked();
	await expect.element(page.getByLabelText('Callback path')).toBeEnabled();
	await expect.element(page.getByLabelText('Callback path')).toHaveValue('/custom');
});
