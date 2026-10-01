import { invalidate } from '$app/navigation';
import {
	Group,
	type GitCredential,
	type ImagePullSecret,
	type ModelProxySettings
} from '$lib/services';
import { errors, productTelemetryConsent, profile } from '$lib/stores';
import { defaultAppNotification } from '$lib/stores/appNotification.svelte';
import { success } from '$lib/stores/success';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import SettingsView from './SettingsView.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		invalidate: vi.fn(async () => {})
	};
});

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	setUrlParamAndUpdateUrl: vi.fn()
}));

const proxyUrl = 'https://model-service.obot.ai';

function renderSettings({
	consent,
	gitCredentials = [],
	imagePullSecrets = [],
	modelProxySettings = { enabled: false, url: proxyUrl },
	showProductAnalytics = true,
	showRegistryConnections = false
}: {
	consent?: boolean;
	gitCredentials?: GitCredential[];
	imagePullSecrets?: ImagePullSecret[];
	modelProxySettings?: ModelProxySettings;
	showProductAnalytics?: boolean;
	showRegistryConnections?: boolean;
} = {}) {
	productTelemetryConsent.initialize({ consent }, true);
	return render(SettingsView, {
		appNotification: defaultAppNotification,
		capability: { available: showRegistryConnections },
		gitCredentials,
		imagePullSecrets,
		modelProxySettings,
		productTelemetryConsent: { consent },
		showProductAnalytics,
		showRegistryConnections
	});
}

describe('Platform settings view', () => {
	beforeEach(async () => {
		await preparePageData();
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.mocked(invalidate).mockClear();
	});

	it('groups the former tabs into named sections with one save and cancel', async () => {
		await renderSettings({ showRegistryConnections: true });

		for (const name of [
			'Notifications',
			'Product Analytics',
			'Model Proxy',
			'Registry Connections',
			'Git Credentials'
		]) {
			await expect.element(page.getByRole('heading', { name, exact: true })).toBeVisible();
		}

		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Cancel', exact: true })).toBeDisabled();
	});

	it('hides sections that are not available', async () => {
		await renderSettings({ showProductAnalytics: false, showRegistryConnections: false });

		await expect
			.element(page.getByRole('heading', { name: 'Product Analytics', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Registry Connections', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Notifications', exact: true }))
			.toBeVisible();
	});

	it.each([
		[false, true],
		[true, false]
	] as const)('saves a product analytics change from %s to %s', async (initial, selected) => {
		const update = vi.fn();
		const successNotification = vi.spyOn(success, 'add');
		worker.use(
			http.put('/api/product-telemetry-consent', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({ consent: selected });
			})
		);

		await renderSettings({ consent: initial });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		const radio = page.getByRole('radio', {
			name: selected ? 'Enable product analytics' : 'Disable product analytics',
			exact: true
		});
		await radio.click();
		await expect.element(save).toBeEnabled();
		await save.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ consent: selected });
			expect(productTelemetryConsent.consent).toBe(selected);
			expect(successNotification).toHaveBeenCalledWith(
				'Product analytics preference updated successfully.'
			);
		});
		await expect.element(radio).toBeChecked();
		await expect.element(save).toBeDisabled();
		successNotification.mockRestore();
	});

	it('cancels an unsaved product analytics choice', async () => {
		await renderSettings({ consent: false });
		const radio = page.getByRole('radio', { name: 'Enable product analytics', exact: true });
		await radio.click();
		await expect.element(radio).toBeChecked();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(radio).not.toBeChecked();
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('retains an unsaved product analytics choice when saving fails', async () => {
		worker.use(
			http.put('/api/product-telemetry-consent', () =>
				HttpResponse.json({ error: 'try again' }, { status: 500 })
			)
		);

		await renderSettings({ consent: false });
		const radio = page.getByRole('radio', { name: 'Enable product analytics', exact: true });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await radio.click();
		await save.click();

		await expect.element(radio).toBeChecked();
		await expect.element(save).toBeEnabled();
	});

	it.each([
		[false, true],
		[true, false]
	] as const)('saves a model proxy change from %s to %s', async (initial, selected) => {
		const update = vi.fn();
		const successNotification = vi.spyOn(success, 'add');
		worker.use(
			http.put('/api/model-proxy', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({ enabled: selected, url: proxyUrl });
			})
		);

		await renderSettings({ modelProxySettings: { enabled: initial, url: proxyUrl } });
		const toggle = page.getByRole('checkbox', { name: /^Enable Model Proxy/ });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await toggle.click();
		await save.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({ enabled: selected });
			expect(invalidate).toHaveBeenCalledWith('model-proxy:usage');
			expect(successNotification).toHaveBeenCalledWith(
				'Model proxy settings updated successfully.'
			);
		});
		await expect.element(save).toBeDisabled();
		successNotification.mockRestore();
	});

	it('creates a git credential from the dialog', async () => {
		const create = vi.fn();
		worker.use(
			http.post('/api/git-credentials', async ({ request }) => {
				create(await request.json());
				return HttpResponse.json({
					id: 'git-1',
					displayName: 'Work',
					host: 'github.com',
					tokenConfigured: true,
					uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] }
				});
			})
		);

		await renderSettings();
		const pageSave = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Add Git Credential', exact: true }).click();
		const dialog = page.getByRole('dialog');
		await expect
			.element(dialog.getByRole('heading', { name: 'Add Git Credential', exact: true }))
			.toBeVisible();
		await expect.element(pageSave).toBeDisabled();

		await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Work');
		await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Add Git Credential', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(pageSave).toBeDisabled();

		await page.getByRole('button', { name: 'Add Git Credential', exact: true }).click();
		await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Work');
		await dialog.getByRole('textbox', { name: 'Git host', exact: true }).fill('github.com');
		await dialog.getByLabelText('Personal access token').fill('ghp_token');
		await dialog.getByRole('button', { name: 'Add', exact: true }).click();

		await expect.element(page.getByText('Work', { exact: true }).first()).toBeVisible();
		await expect.element(pageSave).toBeEnabled();
		expect(create).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(page.getByText('Work', { exact: true })).not.toBeInTheDocument();
		await expect.element(pageSave).toBeDisabled();

		await page.getByRole('button', { name: 'Add Git Credential', exact: true }).click();
		await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Work');
		await dialog.getByRole('textbox', { name: 'Git host', exact: true }).fill('github.com');
		await dialog.getByLabelText('Personal access token').fill('ghp_token');
		await dialog.getByRole('button', { name: 'Add', exact: true }).click();
		await expect.element(pageSave).toBeEnabled();
		await pageSave.click();

		await vi.waitFor(() => {
			expect(create).toHaveBeenCalledWith({
				displayName: 'Work',
				host: 'github.com',
				token: 'ghp_token'
			});
		});
		await expect.element(pageSave).toBeDisabled();
	});

	it('stages an image pull secret in the dialog and saves it with the shared button', async () => {
		const create = vi.fn();
		worker.use(
			http.post('/api/image-pull-secrets', async ({ request }) => {
				const body = await request.json();
				create(body);
				return HttpResponse.json({ id: 'ips-new', manifest: body });
			})
		);

		await renderSettings({ showRegistryConnections: true });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Add Basic Secret', exact: true }).click();
		const dialog = page.getByRole('dialog');
		await expect
			.element(dialog.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
			.toBeVisible();
		await expect.element(save).toBeDisabled();
		await dialog.getByPlaceholder('registry.example.com', { exact: true }).fill('ghcr.io');
		await expect.element(dialog.getByRole('button', { name: 'Add', exact: true })).toBeDisabled();
		await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(save).toBeDisabled();

		await page.getByRole('button', { name: 'Add Basic Secret', exact: true }).click();
		await dialog.getByPlaceholder('registry.example.com', { exact: true }).fill('ghcr.io');
		await dialog.getByPlaceholder('robot-account').fill('obot');
		await dialog.getByPlaceholder('Registry password or token').fill('secret-token');
		await dialog.getByRole('button', { name: 'Add', exact: true }).click();
		await expect.element(page.getByText('ghcr.io', { exact: true }).first()).toBeVisible();
		await expect.element(save).toBeEnabled();
		expect(create).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(page.getByText('ghcr.io', { exact: true })).not.toBeInTheDocument();
		await expect.element(save).toBeDisabled();

		await page.getByRole('button', { name: 'Add Basic Secret', exact: true }).click();
		await dialog.getByPlaceholder('registry.example.com', { exact: true }).fill('ghcr.io');
		await dialog.getByPlaceholder('robot-account').fill('obot');
		await dialog.getByPlaceholder('Registry password or token').fill('secret-token');
		await dialog.getByRole('button', { name: 'Add', exact: true }).click();
		await save.click();

		await vi.waitFor(() => {
			expect(create).toHaveBeenCalledWith({
				enabled: true,
				type: 'basic',
				displayName: '',
				basic: {
					server: 'ghcr.io',
					username: 'obot',
					password: 'secret-token'
				}
			});
		});
		await expect.element(save).toBeDisabled();
	});

	it('edits a staged image pull secret before it is saved', async () => {
		const create = vi.fn();
		worker.use(
			http.post('/api/image-pull-secrets', async ({ request }) => {
				const body = await request.json();
				create(body);
				return HttpResponse.json({ id: 'ips-new', manifest: body });
			})
		);

		await renderSettings({ showRegistryConnections: true });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Add Basic Secret', exact: true }).click();
		const dialog = page.getByRole('dialog');
		await dialog.getByPlaceholder('Production registry', { exact: true }).fill('GHCR');
		await dialog.getByPlaceholder('registry.example.com', { exact: true }).fill('ghcr.io');
		await dialog.getByPlaceholder('robot-account').fill('obot');
		await dialog.getByPlaceholder('Registry password or token').fill('secret-token');
		await dialog.getByRole('button', { name: 'Add', exact: true }).click();

		await page.getByText('GHCR', { exact: true }).first().click();
		await expect
			.element(dialog.getByRole('heading', { name: 'Edit Basic Secret', exact: true }))
			.toBeVisible();
		await expect
			.element(dialog.getByPlaceholder('registry.example.com', { exact: true }))
			.toHaveValue('ghcr.io');
		await dialog.getByPlaceholder('Production registry', { exact: true }).fill('Production GHCR');
		await dialog.getByRole('button', { name: 'Update', exact: true }).click();
		await expect.element(page.getByText('Production GHCR', { exact: true }).first()).toBeVisible();
		await save.click();

		await vi.waitFor(() => {
			expect(create).toHaveBeenCalledWith({
				enabled: true,
				type: 'basic',
				displayName: 'Production GHCR',
				basic: {
					server: 'ghcr.io',
					username: 'obot',
					password: 'secret-token'
				}
			});
		});
	});

	it('enables save when an existing image pull secret is modified', async () => {
		const update = vi.fn();
		worker.use(
			http.put('/api/image-pull-secrets/ips-dockerhub', async ({ request }) => {
				const body = await request.json();
				update(body);
				return HttpResponse.json({
					id: 'ips-dockerhub',
					manifest: body,
					status: { passwordConfigured: true }
				});
			})
		);

		await renderSettings({
			showRegistryConnections: true,
			imagePullSecrets: [
				{
					id: 'ips-dockerhub',
					manifest: {
						enabled: true,
						type: 'basic',
						displayName: 'Docker Hub',
						basic: { server: 'docker.io', username: 'obot' }
					},
					status: { passwordConfigured: true }
				}
			]
		});
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByText('Docker Hub', { exact: true }).first().click();
		const dialog = page.getByRole('dialog');
		await expect
			.element(dialog.getByRole('heading', { name: 'Edit Docker Hub', exact: true }))
			.toBeVisible();
		await expect.element(save).toBeDisabled();
		await dialog.getByPlaceholder('Production registry', { exact: true }).fill('Production');
		await dialog.getByRole('button', { name: 'Update', exact: true }).click();
		await expect.element(page.getByText('Production', { exact: true }).first()).toBeVisible();
		await expect.element(save).toBeEnabled();
		expect(update).not.toHaveBeenCalled();
		await save.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({
				enabled: true,
				type: 'basic',
				displayName: 'Production',
				basic: { server: 'docker.io', username: 'obot' }
			});
		});
		await expect.element(save).toBeDisabled();
	});

	it('updates a git credential from the dialog', async () => {
		const update = vi.fn();
		worker.use(
			http.patch('/api/git-credentials/git-1', async ({ request }) => {
				update(await request.json());
				return HttpResponse.json({
					id: 'git-1',
					displayName: 'Personal',
					host: 'github.com',
					tokenConfigured: true,
					uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] }
				});
			})
		);

		await renderSettings({
			gitCredentials: [
				{
					id: 'git-1',
					displayName: 'Work',
					host: 'github.com',
					tokenConfigured: true,
					uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] }
				}
			]
		});
		const pageSave = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Edit this credential', exact: true }).click();
		const dialog = page.getByRole('dialog');
		await expect
			.element(dialog.getByRole('heading', { name: 'Edit Git Credential', exact: true }))
			.toBeVisible();
		await expect.element(pageSave).toBeDisabled();
		await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Personal');
		await dialog.getByRole('button', { name: 'Update', exact: true }).click();

		await expect.element(page.getByText('Personal', { exact: true }).first()).toBeVisible();
		await expect.element(pageSave).toBeEnabled();
		expect(update).not.toHaveBeenCalled();
		await pageSave.click();

		await vi.waitFor(() => {
			expect(update).toHaveBeenCalledWith({
				displayName: 'Personal',
				host: 'github.com'
			});
		});
		await expect.element(pageSave).toBeDisabled();
	});

	it('deletes an image pull secret when settings are saved', async () => {
		const remove = vi.fn(() => new HttpResponse(null, { status: 204 }));
		worker.use(http.delete('/api/image-pull-secrets/ips-dockerhub', remove));

		await renderSettings({
			showRegistryConnections: true,
			imagePullSecrets: [
				{
					id: 'ips-dockerhub',
					manifest: {
						enabled: true,
						type: 'basic',
						displayName: 'Docker Hub',
						basic: { server: 'docker.io', username: 'obot' }
					},
					status: { passwordConfigured: true }
				}
			]
		});
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Actions for Docker Hub', exact: true }).click();
		await page.getByRole('button', { name: 'Delete', exact: true }).click();
		await expect.element(page.getByText('Docker Hub', { exact: true })).not.toBeInTheDocument();
		await expect.element(save).toBeEnabled();
		expect(remove).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(page.getByText('Docker Hub', { exact: true }).first()).toBeVisible();
		await expect.element(save).toBeDisabled();

		await page.getByRole('button', { name: 'Actions for Docker Hub', exact: true }).click();
		await page.getByRole('button', { name: 'Delete', exact: true }).click();
		await save.click();

		await vi.waitFor(() => {
			expect(remove).toHaveBeenCalledOnce();
		});
		await expect.element(page.getByText('Docker Hub', { exact: true })).not.toBeInTheDocument();
		await expect.element(save).toBeDisabled();
	});

	it('deletes a git credential when settings are saved', async () => {
		const remove = vi.fn(() => new HttpResponse(null, { status: 204 }));
		worker.use(http.delete('/api/git-credentials/git-1', remove));

		await renderSettings({
			gitCredentials: [
				{
					id: 'git-1',
					displayName: 'Work',
					host: 'github.com',
					tokenConfigured: true,
					uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] }
				}
			]
		});
		const pageSave = page.getByRole('button', { name: 'Save', exact: true });
		await page.getByRole('button', { name: 'Delete this credential', exact: true }).click();

		await expect.element(page.getByText('Work', { exact: true })).not.toBeInTheDocument();
		await expect.element(pageSave).toBeEnabled();
		expect(remove).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(page.getByText('Work', { exact: true }).first()).toBeVisible();
		await expect.element(pageSave).toBeDisabled();

		await page.getByRole('button', { name: 'Delete this credential', exact: true }).click();
		await expect.element(pageSave).toBeEnabled();
		await pageSave.click();

		await vi.waitFor(() => {
			expect(remove).toHaveBeenCalledOnce();
		});
		await expect.element(page.getByText('Work', { exact: true })).not.toBeInTheDocument();
		await expect.element(pageSave).toBeDisabled();
	});

	it('keeps a git credential when delete conflicts', async () => {
		const append = vi.spyOn(errors, 'append');
		worker.use(
			http.delete('/api/git-credentials/git-1', () =>
				HttpResponse.json({ message: 'in use' }, { status: 409 })
			)
		);

		await renderSettings({
			gitCredentials: [
				{
					id: 'git-1',
					displayName: 'Work',
					host: 'github.com',
					tokenConfigured: true,
					uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] }
				}
			]
		});
		await page.getByRole('button', { name: 'Delete this credential', exact: true }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(append.mock.calls.flat().join('\n')).toContain('409');
		});
		await expect.element(page.getByText('In Use', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Delete this credential', exact: true }))
			.toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
		append.mockRestore();
	});

	it('does not save other sections when the notification banner is invalid', async () => {
		const updateConsent = vi.fn();
		worker.use(
			http.put('/api/product-telemetry-consent', async ({ request }) => {
				updateConsent(await request.json());
				return HttpResponse.json({ consent: true });
			})
		);

		await renderSettings({ consent: false });
		await page.getByRole('checkbox', { name: /Enable Banner/ }).click();
		await page.getByRole('radio', { name: 'Enable product analytics', exact: true }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await expect.element(page.getByText('This field is required.', { exact: true })).toBeVisible();
		expect(updateConsent).not.toHaveBeenCalled();
	});
});
