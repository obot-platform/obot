import type { APIKey } from '$lib/services';
import GenerateApiKeyButton from './GenerateApiKeyButton.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function key(overrides: Partial<APIKey> = {}): APIKey {
	return {
		id: 1,
		userId: 1,
		name: 'Agent Identity API Key',
		canAccessAPI: false,
		canAccessLLMProxy: true,
		canAccessSkills: false,
		canAccessDeviceScans: false,
		createdAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

describe('GenerateApiKeyButton', () => {
	it('generates a key when none exist', async () => {
		const onGenerate = vi.fn();
		render(GenerateApiKeyButton, { onGenerate });

		await page.getByRole('button', { name: 'Generate Key' }).click();

		expect(onGenerate).toHaveBeenCalledOnce();
		await expect
			.element(page.getByRole('link', { name: 'Manage API Keys' }))
			.not.toBeInTheDocument();
	});

	it('offers management and a new key when a usable LLM key exists', async () => {
		const onGenerate = vi.fn();
		render(GenerateApiKeyButton, {
			apiKeys: [key({ expiresAt: '2999-01-01T00:00:00.000Z' })],
			onGenerate
		});

		const link = page.getByRole('link', { name: 'Manage API Keys' });
		await expect.element(link).toBeVisible();
		await expect
			.element(link)
			.toHaveAttribute('href', expect.stringContaining('/identity-access?view=agents'));
		await expect
			.element(page.getByRole('button', { name: 'Generate Key', exact: true }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Row actions' }).click();
		await page.getByRole('button', { name: 'Generate New Key' }).click();

		expect(onGenerate).toHaveBeenCalledOnce();
	});

	it('ignores expired and non-LLM keys', async () => {
		render(GenerateApiKeyButton, {
			onGenerate: vi.fn(),
			apiKeys: [
				key({ expiresAt: '2000-01-01T00:00:00.000Z' }),
				key({ id: 2, canAccessLLMProxy: false, expiresAt: '2999-01-01T00:00:00.000Z' }),
				key({ id: 3, expiresAt: 'not-a-date' })
			]
		});

		await expect.element(page.getByRole('button', { name: 'Generate Key' })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Manage API Keys' }))
			.not.toBeInTheDocument();
	});

	it('treats a key created in this session as existing', async () => {
		render(GenerateApiKeyButton, { onGenerate: vi.fn(), hasGeneratedKey: true });

		await expect.element(page.getByRole('link', { name: 'Manage API Keys' })).toBeVisible();
	});

	it('disables generate while a key is being created', async () => {
		render(GenerateApiKeyButton, { onGenerate: vi.fn(), generating: true });

		await expect.element(page.getByRole('button')).toBeDisabled();
	});
});
