import { Group } from '$lib/services';
import type { MDMDevice } from '$lib/services/admin/types';
import { profile, version } from '$lib/stores';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import Devices from './Devices.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function device(overrides: Partial<MDMDevice> = {}): MDMDevice {
	return {
		id: 1,
		deviceID: 'device-1-0123456789',
		mdmConfigurationID: 1,
		hostname: 'host-1',
		os: 'darwin',
		osVersion: '15.1',
		enrolledAt: new Date(Date.now() - 86_400_000).toISOString(),
		...overrides
	};
}

function mockDevices(devices: MDMDevice[]) {
	worker.use(http.get('/api/mdm/devices', () => HttpResponse.json({ items: devices })));
}

function renderDevices({ groups = [Group.ADMIN] }: { groups?: string[] } = {}) {
	profile.initialize(createMockProfile(groups));
	version.initialize();
	return render(Devices);
}

function row(hostname: string) {
	return page.getByRole('row').filter({ hasText: hostname });
}

describe('Devices', () => {
	it('lists enrolled devices with their latest scan', async () => {
		mockDevices([
			device({
				latestScan: {
					id: 10,
					scannedAt: new Date(Date.now() - 3_600_000).toISOString(),
					username: 'alice',
					arch: 'arm64',
					mcpServerCount: 7,
					skillCount: 3,
					pluginCount: 0,
					clientCount: 2
				}
			}),
			device({ id: 2, deviceID: 'device-2-0123456789', hostname: 'host-2' })
		]);

		renderDevices();

		await expect.element(page.getByText('host-1')).toBeVisible();
		await expect.element(page.getByText('darwin 15.1 / arm64')).toBeVisible();
		await expect.element(page.getByText('alice')).toBeVisible();
		await expect.element(page.getByText('7', { exact: true })).toBeVisible();
		await expect.element(page.getByText('host-2')).toBeVisible();
		await expect.element(page.getByText('Never', { exact: true })).toBeVisible();
	});

	it('shows an empty state when no devices are enrolled', async () => {
		mockDevices([]);

		renderDevices();

		await expect.element(page.getByText('No devices enrolled yet')).toBeVisible();
	});

	it('removes a device after confirmation', async () => {
		const deleted = vi.fn();
		mockDevices([device(), device({ id: 2, deviceID: 'device-2-0123456789', hostname: 'host-2' })]);
		worker.use(
			http.delete('/api/mdm/devices/:deviceID', ({ params, request }) => {
				deleted(params.deviceID, new URL(request.url).searchParams.get('deleteScans'));
				return new HttpResponse(null, { status: 204 });
			})
		);

		renderDevices();

		await row('host-1').getByRole('button', { name: 'Row actions' }).click();
		await page.getByRole('button', { name: 'Remove', exact: true }).click();

		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByText('Remove host-1?')).toBeVisible();
		await dialog.getByRole('button', { name: 'Remove', exact: true }).click();

		await expect.element(row('host-1')).not.toBeInTheDocument();
		await expect.element(row('host-2')).toBeVisible();
		expect(deleted).toHaveBeenCalledExactlyOnceWith('device-1-0123456789', 'true');
	});

	it('retries only the devices that failed to be removed', async () => {
		const deleted = vi.fn();
		let failedOnce = false;
		mockDevices([
			device(),
			device({ id: 2, deviceID: 'device-2-0123456789', hostname: 'host-2' }),
			device({ id: 3, deviceID: 'device-3-0123456789', hostname: 'host-3' })
		]);
		worker.use(
			http.delete('/api/mdm/devices/:deviceID', ({ params }) => {
				deleted(params.deviceID);
				if (params.deviceID === 'device-2-0123456789' && !failedOnce) {
					failedOnce = true;
					return HttpResponse.json({ error: 'boom' }, { status: 500 });
				}
				return new HttpResponse(null, { status: 204 });
			})
		);

		renderDevices();

		await row('host-1').getByRole('button').first().click();
		await row('host-2').getByRole('button').first().click();
		await expect.element(page.getByText('2 of 3 selected')).toBeVisible();
		await page.getByRole('button', { name: 'Remove', exact: true }).click();

		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByText('Remove 2 devices?')).toBeVisible();
		await dialog.getByRole('button', { name: 'Remove', exact: true }).click();

		// host-1 is removed, host-2 fails, and only host-2 is left to retry
		await expect.element(dialog.getByText('Remove host-2?')).toBeVisible();
		await expect.element(row('host-1')).not.toBeInTheDocument();
		await dialog.getByRole('button', { name: 'Remove', exact: true }).click();

		await expect.element(row('host-2')).not.toBeInTheDocument();
		await expect.element(row('host-3')).toBeVisible();
		expect(deleted.mock.calls.map(([id]) => id)).toEqual([
			'device-1-0123456789',
			'device-2-0123456789',
			'device-2-0123456789'
		]);
		await expect.element(page.getByText(/selected$/)).not.toBeInTheDocument();
	});

	it('hides remove actions from read-only admins', async () => {
		mockDevices([device()]);

		renderDevices({ groups: [Group.AUDITOR] });

		await expect.element(page.getByText('host-1')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Row actions' })).not.toBeInTheDocument();
	});
});
