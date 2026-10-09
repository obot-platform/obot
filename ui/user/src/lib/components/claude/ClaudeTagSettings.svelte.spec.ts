import { m } from '$lib/i18n';
import { setLocale } from '$lib/paraglide/runtime';
import '$lib/services';
import { worker } from '../../../tests/mocks/worker';
import ClaudeTagSettings from './ClaudeTagSettings.svelte';
import type { ClaudeEnvironment } from './types';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => setLocale('en', { reload: false }));

it('enables a new Tag identity from a localized UI', async () => {
	setLocale('ja', { reload: false });
	const saved = vi.fn();
	const requestBody = vi.fn();
	worker.use(
		http.put('*/api/claude-environments/env/claude-tag', async ({ request }) => {
			requestBody(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeTagSettings, {
		environment: { id: 'env', displayName: 'Test', claudeTagEnabled: false } as ClaudeEnvironment,
		onsaved: saved
	});
	await expect.element(page.getByText(m.hosted_agents_tag_help())).toBeVisible();
	await page.getByRole('button', { name: m.hosted_agents_tag_create() }).click();
	await expect.element(page.getByText(m.hosted_agents_tag_saved())).toBeVisible();
	expect(requestBody).toHaveBeenCalledWith({ enabled: true });
	expect(saved).toHaveBeenCalledOnce();
});

it('disables an existing Tag identity', async () => {
	const requestBody = vi.fn();
	worker.use(
		http.put('*/api/claude-environments/env/claude-tag', async ({ request }) => {
			requestBody(await request.json());
			return HttpResponse.json({});
		})
	);
	render(ClaudeTagSettings, {
		environment: {
			id: 'env',
			displayName: 'Test',
			claudeTagEnabled: true,
			claudeTagUserID: '123'
		} as ClaudeEnvironment,
		onsaved: vi.fn()
	});
	await page.getByRole('checkbox', { name: m.hosted_agents_tag_enable() }).click();
	await page.getByRole('button', { name: m.hosted_agents_tag_save() }).click();
	await expect.element(page.getByText(m.hosted_agents_tag_saved())).toBeVisible();
	expect(requestBody).toHaveBeenCalledWith({ enabled: false });
});
