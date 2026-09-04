import type {
	GitCredential,
	VMcpRepository,
	VMcpRepositoryManifest
} from '$lib/services/admin/types';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcpSource from './CreateEditVMcpSource.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createGitCredential(overrides: Partial<GitCredential> = {}): GitCredential {
	return {
		id: 'cred-1',
		displayName: 'GitHub PAT',
		host: 'github.com',
		tokenConfigured: true,
		uses: { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] },
		...overrides
	};
}

function createRepository(overrides: Partial<VMcpRepository> = {}): VMcpRepository {
	return {
		id: 'repo-1',
		created: '2026-01-01T00:00:00.000Z',
		displayName: 'vMCP Sources',
		repoURL: 'https://github.com/org/vmcps',
		ref: 'main',
		isSyncing: false,
		discoveredVMcpCount: 0,
		...overrides
	};
}

async function renderSourceDialog(
	props: {
		gitCredentials?: GitCredential[];
		vmcpRepositories?: VMcpRepository[];
		onSaved?: (repository: VMcpRepository) => void;
	} = {}
) {
	await preparePageData();
	return render(CreateEditVMcpSource, {
		gitCredentials: props.gitCredentials ?? [],
		vmcpRepositories: props.vmcpRepositories ?? [],
		onSaved: props.onSaved
	});
}

describe('CreateEditVMcpSource.svelte', () => {
	it('opens the add dialog with a default main ref', async () => {
		const result = await renderSourceDialog();
		result.component.openAdd();

		await expect.element(page.getByText('Add Source URL')).toBeVisible();
		await expect.element(page.getByRole('textbox', { name: 'Name' })).toHaveValue('');
		await expect.element(page.getByRole('textbox', { name: 'Source URL' })).toHaveValue('');
		await expect.element(page.getByRole('textbox', { name: 'Reference' })).toHaveValue('main');
		await expect.element(page.getByRole('button', { name: 'Add' })).toBeEnabled();
	});

	it('creates a source URL and reports the saved repository', async () => {
		const saved = createRepository();
		const onSaved = vi.fn();
		const createRequest = vi.fn();

		worker.use(
			http.post('/api/vmcp-repositories', async ({ request }) => {
				const manifest = (await request.json()) as VMcpRepositoryManifest;
				createRequest(manifest);
				return HttpResponse.json({ ...saved, ...manifest });
			})
		);

		const result = await renderSourceDialog({ onSaved });
		result.component.openAdd();

		await page.getByRole('textbox', { name: 'Name' }).fill('vMCP Sources');
		await page.getByRole('textbox', { name: 'Source URL' }).fill('https://github.com/org/vmcps');
		await page.getByRole('button', { name: 'Add' }).click();

		await vi.waitFor(() => expect(createRequest).toHaveBeenCalled());
		expect(createRequest.mock.calls[0][0]).toMatchObject({
			displayName: 'vMCP Sources',
			repoURL: 'https://github.com/org/vmcps',
			ref: 'main'
		});
		await vi.waitFor(() => expect(onSaved).toHaveBeenCalled());
		await expect.element(page.getByText('Add Source URL')).not.toBeInTheDocument();
	});

	it('opens an existing source for edit and saves updates', async () => {
		const repository = createRepository();
		const onSaved = vi.fn();
		const updateRequest = vi.fn();

		worker.use(
			http.put(`/api/vmcp-repositories/${repository.id}`, async ({ request }) => {
				const manifest = (await request.json()) as VMcpRepositoryManifest;
				updateRequest(manifest);
				return HttpResponse.json({ ...repository, ...manifest });
			})
		);

		const result = await renderSourceDialog({
			vmcpRepositories: [repository],
			onSaved
		});
		result.component.openEdit(repository);

		await expect.element(page.getByText('Edit Source URL')).toBeVisible();
		await expect
			.element(page.getByRole('textbox', { name: 'Name' }))
			.toHaveValue(repository.displayName);
		await expect
			.element(page.getByRole('textbox', { name: 'Source URL' }))
			.toHaveValue(repository.repoURL);

		await page.getByRole('textbox', { name: 'Name' }).fill('Renamed Sources');
		await page.getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() => expect(updateRequest).toHaveBeenCalled());
		expect(updateRequest.mock.calls[0][0].displayName).toBe('Renamed Sources');
		await vi.waitFor(() => expect(onSaved).toHaveBeenCalled());
	});

	it('requires a shared git credential before adding', async () => {
		const result = await renderSourceDialog({ gitCredentials: [createGitCredential()] });
		result.component.openAdd();

		await page.getByRole('combobox', { name: 'Credential' }).click();
		await page.getByRole('button', { name: 'Choose existing' }).click();

		await expect.element(page.getByRole('button', { name: 'Add' })).toBeDisabled();
		await expect
			.element(page.getByText('Only credentials matching the repository host can be selected.'))
			.toBeVisible();
	});
});
