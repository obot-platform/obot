import { page as appPage } from '$app/state';
import type { ImagePullSecret, ImagePullSecretCapability } from '$lib/services';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import RegistryConnectionsView from './RegistryConnectionsView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	setUrlParamAndUpdateUrl: vi.fn()
}));

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
	it('shows an empty state when no secrets exist', async () => {
		await renderRegistryConnections();

		await expect
			.element(page.getByText("Click '+' to add a basic secret.", { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Basic Secret', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
			.not.toBeInTheDocument();
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
			.element(page.getByRole('button', { name: 'Add Basic Secret', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add ECR Secret', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
			.not.toBeInTheDocument();
	});

	it('lists existing secrets', async () => {
		await renderRegistryConnections({ imagePullSecrets: [dockerHubSecret] });

		await expect.element(page.getByText('Basic Secrets', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Docker Hub', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByText('docker.io', { exact: true }).first()).toBeVisible();
	});

	it('opens the editor when the query id matches a secret', async () => {
		await renderRegistryConnections({
			imagePullSecrets: [dockerHubSecret],
			id: dockerHubSecret.id
		});

		await expect
			.element(page.getByRole('heading', { name: 'Edit Docker Hub', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('Registry Server', { exact: true })).toBeVisible();
	});

	it('opens the create form from the query and keeps the list', async () => {
		await renderRegistryConnections({ create: true });

		await expect
			.element(page.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('Registry Server', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText("Click '+' to add a basic secret.", { exact: true }))
			.toBeVisible();
	});

	it('opens an ECR form from the ECR section button', async () => {
		await renderRegistryConnections();

		await page.getByRole('button', { name: 'Add ECR Secret', exact: true }).click();

		await expect
			.element(page.getByRole('heading', { name: 'Add ECR Secret', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('Role ARN', { exact: true })).toBeVisible();
	});

	it('stages a secret deletion without calling the api', async () => {
		const deleteSecret = vi.fn(() => new HttpResponse(null, { status: 204 }));
		worker.use(http.delete('/api/image-pull-secrets/ips-dockerhub', deleteSecret));

		await renderRegistryConnections({ imagePullSecrets: [dockerHubSecret] });

		await page.getByRole('button', { name: 'Actions for Docker Hub', exact: true }).click();
		await page.getByRole('button', { name: 'Delete', exact: true }).click();

		await expect.element(page.getByText('Docker Hub', { exact: true })).not.toBeInTheDocument();
		expect(deleteSecret).not.toHaveBeenCalled();
	});
});
