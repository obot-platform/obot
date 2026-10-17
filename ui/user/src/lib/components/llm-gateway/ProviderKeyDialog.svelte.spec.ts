import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import ProviderKeyDialog from './ProviderKeyDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';

function mockCreateKey(status = 200) {
	const created = vi.fn();
	worker.use(
		http.post('/api/llm-gateway/admin/providers/:name/keys', async ({ params, request }) => {
			created(params.name, await request.json());
			return status === 200
				? HttpResponse.json({ id: 'new-key', name: 'saved' })
				: HttpResponse.json({ error: { message: 'key name already exists' } }, { status });
		})
	);
	return created;
}

async function openDialog(props: {
	provider: string;
	baseProvider?: string;
	existingKeyNames?: string[];
}) {
	const onSaved = vi.fn();
	const dialog = await renderOpenDialog(ProviderKeyDialog, { ...props, onSaved });
	return { dialog, onSaved };
}

describe('ProviderKeyDialog', () => {
	it('suggests the first unused key name and requires the API key', async () => {
		const created = mockCreateKey();
		const { dialog, onSaved } = await openDialog({
			provider: 'openai',
			existingKeyNames: ['openai-key-1', 'openai-key-2']
		});

		await expect.element(dialog.getByLabelText('Name')).toHaveValue('openai-key-3');
		const save = dialog.getByRole('button', { name: 'Save', exact: true });
		await expect.element(save).toBeDisabled();

		await dialog.getByPlaceholder('sk-...').fill('sk-live');
		await dialog.getByLabelText('Blocked models').fill('o1-pro, o3');
		await save.click();

		await vi.waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith('openai', {
			name: 'openai-key-3',
			models: ['*'],
			blacklisted_models: ['o1-pro', 'o3'],
			weight: 1,
			enabled: true,
			value: 'sk-live'
		});
	});

	it('nests provider-specific fields and sends an empty value for providers without an API key', async () => {
		const created = mockCreateKey();
		const { dialog, onSaved } = await openDialog({ provider: 'vertex' });

		await dialog.getByLabelText(/^Project ID/).fill('my-project');
		await dialog.getByLabelText(/^Region/).fill('us-east5');
		await dialog.getByRole('switch', { name: 'Enabled' }).click();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith('vertex', {
			name: 'vertex-key-1',
			models: ['*'],
			blacklisted_models: [],
			weight: 1,
			enabled: false,
			value: '',
			vertex_key_config: { project_id: 'my-project', region: 'us-east5' }
		});
	});

	it('uses the base provider fields for a custom provider', async () => {
		const created = mockCreateKey();
		const { dialog, onSaved } = await openDialog({ provider: 'my-llm', baseProvider: 'openai' });

		await expect.element(dialog.getByLabelText('Name')).toHaveValue('my-llm-key-1');
		await dialog.getByPlaceholder('sk-...').fill('sk-custom');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith(
			'my-llm',
			expect.objectContaining({ name: 'my-llm-key-1', value: 'sk-custom' })
		);
	});

	it('shows the server error and keeps the dialog open', async () => {
		mockCreateKey(409);
		const { dialog, onSaved } = await openDialog({ provider: 'openai' });

		await dialog.getByPlaceholder('sk-...').fill('sk-live');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await expect.element(dialog.getByText('key name already exists')).toBeInTheDocument();
		expect(onSaved).not.toHaveBeenCalled();
	});
});
