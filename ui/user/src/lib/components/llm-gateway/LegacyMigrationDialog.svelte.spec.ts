import { Group } from '$lib/services';
import type { MigrationStatus } from '$lib/services/llm-gateway/gateway';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import LegacyMigrationDialog from './LegacyMigrationDialog.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function pending(): MigrationStatus {
	return {
		version: 3,
		choices: {},
		mappings: {},
		sources: {
			'azure-model-provider': {
				provider: 'azure',
				status: 'pending',
				endpoint: 'https://one.openai.azure.com',
				models: ['gpt-5', 'gpt-5-mini']
			},
			'azure-entra-model-provider': {
				provider: 'azure',
				status: 'pending',
				endpoint: 'https://two.services.ai.azure.com',
				models: ['claude-sonnet-4-5']
			}
		}
	};
}
function mockMigration(fail = false) {
	const read = vi.fn();
	const saved = vi.fn();
	worker.use(
		http.get('/api/llm-gateway/migration', () => {
			read();
			return HttpResponse.json(pending());
		}),
		http.post('/api/llm-gateway/migration/resolve', async ({ request }) => {
			const body = (await request.json()) as { family: string; source: string; version: number };
			saved(body);
			if (fail)
				return HttpResponse.json(
					{ error: 'The migration decision changed; refresh and try again' },
					{ status: 409 }
				);
			const state = pending();
			state.version++;
			state.choices[body.family] = body.source;
			return HttpResponse.json(state);
		})
	);
	return { read, saved };
}
const title = 'Choose a model provider to migrate';
beforeEach(() => profile.initialize(createMockProfile([Group.ADMIN])));

describe('LegacyMigrationDialog', () => {
	it('requires an explicit choice and sends the versioned selection', async () => {
		const { saved } = mockMigration();
		render(LegacyMigrationDialog);
		const dialog = page.getByRole('dialog', { name: title });
		await expect.element(dialog).toBeVisible();
		const apply = dialog.getByRole('button', { name: 'Migrate selected provider' });
		await expect.element(apply).toBeDisabled();
		await expect.element(dialog.getByText('https://two.services.ai.azure.com')).toBeVisible();
		await dialog.getByRole('radio').nth(1).click();
		await apply.click();
		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({
				family: 'azure',
				source: 'azure-entra-model-provider',
				version: 3
			})
		);
		await expect.element(dialog).not.toBeInTheDocument();
	});
	it('keeps models collapsed until expanded without selecting a provider', async () => {
		mockMigration();
		render(LegacyMigrationDialog);
		const dialog = page.getByRole('dialog', { name: title });
		await expect.element(dialog).toBeVisible();
		await expect.element(dialog.getByText('gpt-5', { exact: true })).not.toBeVisible();
		const disclosure = dialog.getByText('Models (2)', { exact: true });
		await disclosure.click();
		await expect.element(dialog.getByText('gpt-5', { exact: true })).toBeVisible();
		expect(dialog.getByRole('listitem').elements()).toHaveLength(2);
		await expect.element(dialog.getByRole('radio').first()).not.toBeChecked();
		await expect.element(dialog.getByText('claude-sonnet-4-5', { exact: true })).not.toBeVisible();
		await disclosure.click();
		await expect.element(dialog.getByText('gpt-5', { exact: true })).not.toBeVisible();
	});
	it('defers for the browser session and can be reopened', async () => {
		const { saved } = mockMigration();
		render(LegacyMigrationDialog);
		const dialog = page.getByRole('dialog', { name: title });
		await expect.element(dialog).toBeVisible();
		await dialog.getByRole('button', { name: 'Decide later' }).click();
		expect(sessionStorage.getItem('obot-legacy-migration-deferred')).toBe('true');
		expect(saved).not.toHaveBeenCalled();
		window.dispatchEvent(new Event('open-llm-migration'));
		await expect.element(dialog).toBeVisible();
	});
	it('keeps a stale submission open and reloads the choices', async () => {
		const { read } = mockMigration(true);
		render(LegacyMigrationDialog);
		const dialog = page.getByRole('dialog', { name: title });
		await expect.element(dialog).toBeVisible();
		await dialog.getByRole('radio').first().click();
		await dialog.getByRole('button', { name: 'Migrate selected provider' }).click();
		await expect.element(dialog.getByRole('alert')).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Migrate selected provider' }))
			.toBeDisabled();
		await vi.waitFor(() => expect(read.mock.calls.length).toBeGreaterThanOrEqual(2));
	});
	it('does not fetch migration details for ordinary users or auditors', async () => {
		const { read } = mockMigration();
		profile.initialize(createMockProfile([Group.AUDITOR]));
		render(LegacyMigrationDialog);
		window.dispatchEvent(new Event('open-llm-migration'));
		await expect.element(page.getByRole('dialog', { name: title })).not.toBeInTheDocument();
		expect(read).not.toHaveBeenCalled();
	});
	it('does not automatically reopen a deferred decision', async () => {
		const { read } = mockMigration();
		sessionStorage.setItem('obot-legacy-migration-deferred', 'true');
		render(LegacyMigrationDialog);
		await vi.waitFor(() => expect(read).toHaveBeenCalled());
		await expect.element(page.getByRole('dialog', { name: title })).not.toBeInTheDocument();
	});
});
