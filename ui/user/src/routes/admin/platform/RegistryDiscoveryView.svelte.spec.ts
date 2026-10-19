import { Group } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import RegistryDiscoveryView from './RegistryDiscoveryView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Registry discovery settings', () => {
	it('locks the toggle during a Platform save while keeping the risk notice visible', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		await render(RegistryDiscoveryView, {
			settings: { anonymousAccessEnabled: true },
			saving: true
		});
		const toggle = page.getByRole('checkbox', { name: 'Allow anonymous registry discovery' });
		await expect.element(toggle).toBeChecked();
		await expect.element(toggle).toBeDisabled();
		await expect
			.element(page.getByText('Anonymous discovery publishes server metadata'))
			.toBeVisible();
	});
});

describe('Registry settings refresh', () => {
	const toggle = () => page.getByRole('checkbox', { name: 'Allow anonymous registry discovery' });

	it('recovers from unavailable settings with the saved value and remains clean', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		const update = vi.fn(async ({ request }: { request: Request }) =>
			HttpResponse.json(await request.json())
		);
		worker.use(http.put('/api/registry-settings', update));
		const view = await render(RegistryDiscoveryView, {});
		await expect.element(page.getByRole('alert')).toBeVisible();
		await view.rerender({ settings: { anonymousAccessEnabled: true } });
		await expect.element(toggle()).toBeChecked();
		expect(await view.component.save()).toBe(false);
		expect(update).not.toHaveBeenCalled();
		await toggle().click();
		expect(await view.component.save()).toBe(true);
		await expect.element(toggle()).not.toBeChecked();
	});

	it('updates a clean toggle when saved settings change', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		const view = await render(RegistryDiscoveryView, {
			settings: { anonymousAccessEnabled: false }
		});
		await view.rerender({ settings: { anonymousAccessEnabled: true } });
		await expect.element(toggle()).toBeChecked();
		await view.rerender({ settings: { anonymousAccessEnabled: false } });
		await expect.element(toggle()).not.toBeChecked();
	});

	it('preserves unsaved edits through a reload and does not revert a successful save', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		const update = vi.fn();
		worker.use(
			http.put('/api/registry-settings', async ({ request }) => {
				const body = await request.json();
				update(body);
				return HttpResponse.json(body);
			})
		);
		const view = await render(RegistryDiscoveryView, {
			settings: { anonymousAccessEnabled: true }
		});
		await toggle().click();
		await view.rerender({ settings: undefined });
		await view.rerender({ settings: { anonymousAccessEnabled: true } });
		await expect.element(toggle()).not.toBeChecked();
		expect(await view.component.save()).toBe(true);
		expect(update).toHaveBeenCalledWith({ anonymousAccessEnabled: false });
		await expect.element(toggle()).not.toBeChecked();
		expect(await view.component.save()).toBe(false);
	});
});
