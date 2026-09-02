import { page as appPage } from '$app/state';
import type { ImagePullSecret, ImagePullSecretCapability } from '$lib/services';
import * as url from '$lib/url';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import RegistryConnectionsView from './RegistryConnectionsView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), { spy: true });

const availableCapability: ImagePullSecretCapability = { available: true };
const unavailableCapability: ImagePullSecretCapability = {
	available: false,
	reason: 'Kubernetes is required for managed image pull secrets'
};

const dockerHubSecret: ImagePullSecret = {
	id: 'ips-dockerhub',
	manifest: {
		enabled: true,
		type: 'basic',
		displayName: 'Docker Hub',
		basic: { server: 'docker.io', username: 'obot' }
	},
	status: { passwordConfigured: true }
};

function mockImagePullSecretApis({
	capability = availableCapability,
	items = [] as ImagePullSecret[]
} = {}) {
	worker.use(
		http.get('/api/image-pull-secrets/capability', () => HttpResponse.json(capability)),
		http.get('/api/image-pull-secrets', () => HttpResponse.json({ items }))
	);
}

async function renderRegistryConnections({
	capability = availableCapability,
	imagePullSecrets = [] as ImagePullSecret[],
	create = false,
	id
}: {
	capability?: ImagePullSecretCapability;
	imagePullSecrets?: ImagePullSecret[];
	create?: boolean;
	id?: string;
} = {}) {
	appPage.url.searchParams.set('view', 'registry-connections');
	if (create) {
		appPage.url.searchParams.set('create', 'true');
	}
	if (id) {
		appPage.url.searchParams.set('id', id);
	}

	await preparePageData();
	return render(RegistryConnectionsView, { capability, imagePullSecrets });
}

afterEach(() => {
	appPage.url.searchParams.delete('view');
	appPage.url.searchParams.delete('create');
	appPage.url.searchParams.delete('id');
});

describe('RegistryConnectionsView', () => {
	beforeEach(() => {
		vi.mocked(url.goto).mockImplementation(() => undefined as never);
	});

	afterEach(() => {
		vi.mocked(url.goto).mockReset();
	});
	it('shows an empty state when no secrets exist', async () => {
		await renderRegistryConnections();

		await expect.element(page.getByText('No image pull secrets', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Create New Secret', exact: true }))
			.toBeVisible();
	});

	it('shows a capability banner and hides create when unavailable', async () => {
		await renderRegistryConnections({ capability: unavailableCapability });

		await expect
			.element(page.getByText('Managed image pull secrets are unavailable.', { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText(unavailableCapability.reason!, { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Create New Secret', exact: true }))
			.not.toBeInTheDocument();
	});

	it('lists existing secrets', async () => {
		await renderRegistryConnections({ imagePullSecrets: [dockerHubSecret] });

		await expect.element(page.getByText('Basic Secrets', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Docker Hub', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByText('docker.io', { exact: true }).first()).toBeVisible();
	});

	it('shows required errors when creating without fields', async () => {
		await renderRegistryConnections({ create: true });

		await expect.element(page.getByText('Registry Server', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Create', exact: true }).click();

		await expect
			.element(page.getByText('Registry Server is required', { exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('Username is required', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Password is required', { exact: true })).toBeVisible();
	});

	it('creates a basic secret', async () => {
		const createSecret = vi.fn();
		mockImagePullSecretApis();
		worker.use(
			http.post('/api/image-pull-secrets', async ({ request }) => {
				const body = await request.json();
				createSecret(body);
				return HttpResponse.json({
					id: 'ips-new',
					manifest: body
				});
			})
		);

		await renderRegistryConnections({ create: true });

		await page.getByPlaceholder('registry.example.com', { exact: true }).fill('ghcr.io');
		await page.getByPlaceholder('robot-account').fill('obot');
		await page.getByPlaceholder('Registry password or token').fill('secret-token');
		await page.getByRole('button', { name: 'Create', exact: true }).click();

		await vi.waitFor(() => {
			expect(createSecret).toHaveBeenCalledWith({
				enabled: true,
				type: 'basic',
				displayName: '',
				basic: {
					server: 'ghcr.io',
					username: 'obot',
					password: 'secret-token'
				}
			});
			expect(url.goto).toHaveBeenCalledWith('/admin/platform?view=registry-connections', {
				replaceState: true,
				noScroll: true
			});
		});
	});

	it('deletes a secret from the list', async () => {
		const deleteSecret = vi.fn(() => new HttpResponse(null, { status: 204 }));
		worker.use(http.delete('/api/image-pull-secrets/ips-dockerhub', deleteSecret));

		await renderRegistryConnections({ imagePullSecrets: [dockerHubSecret] });

		await page.getByRole('button', { name: 'Actions for Docker Hub', exact: true }).click();
		await page.getByRole('button', { name: 'Delete', exact: true }).click();
		await expect.element(page.getByText('Delete Docker Hub?', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();

		await vi.waitFor(() => {
			expect(deleteSecret).toHaveBeenCalledOnce();
		});
		await expect.element(page.getByText('Docker Hub', { exact: true })).not.toBeInTheDocument();
	});
});
