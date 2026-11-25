import { goto } from '$lib/url';
import { worker } from '../../../tests/mocks/worker';
import MCPTunnelForm from './MCPTunnelForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

afterEach(() => {
	vi.mocked(goto).mockClear();
});

const tunnel = {
	id: 'tun1',
	created: '2026-01-01T00:00:00Z',
	token: 'tok1-*****',
	manifest: { displayName: 'Office', description: '', allowedURLs: ['https://a.example.com'] }
};

describe('MCPTunnelForm', () => {
	it('keeps Create enabled and, on click, flags and focuses the first invalid field', async () => {
		const onCreate = vi.fn();
		render(MCPTunnelForm, { onCreate });

		const create = page.getByRole('button', { name: 'Create Tunnel' });
		await expect.element(create).toBeEnabled();

		await page.getByRole('button', { name: 'Allowed URL', exact: true }).click();
		await create.click();

		const displayName = page.getByRole('textbox', { name: 'Display Name' });
		await expect.element(displayName).toHaveAttribute('aria-invalid', 'true');
		await expect.element(displayName).toHaveFocus();
		expect(onCreate).not.toHaveBeenCalled();

		// Fixing a field clears its highlight without clearing the others.
		await displayName.fill('Office');
		await expect.element(displayName).not.toHaveAttribute('aria-invalid');
		await expect.element(page.getByText('Allowed URL is required', { exact: false })).toBeVisible();
	});

	it('saves an existing tunnel without disabling Save first', async () => {
		const updated = vi.fn();
		worker.use(
			http.put('/api/mcp-tunnels/tun1', async ({ request }) => {
				const manifest = await request.json();
				updated(manifest);
				return HttpResponse.json({ ...tunnel, manifest });
			})
		);
		const onUpdate = vi.fn();
		render(MCPTunnelForm, { tunnel, onUpdate });

		const save = page.getByRole('button', { name: 'Save Changes' });
		await expect.element(save).toBeEnabled();
		await save.click();

		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
	});

	it('cancels to the tunnels list', async () => {
		render(MCPTunnelForm, { tunnel });

		await page.getByRole('button', { name: 'Cancel' }).click();

		expect(goto).toHaveBeenCalledWith('/mcp-servers?view=tunnels');
	});
});
