import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import AddProviderDialog from './AddProviderDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

function mockCreate({ keyStatus = 200 }: { keyStatus?: number } = {}) {
	const created = vi.fn();
	const keyCreated = vi.fn();
	const deleted = vi.fn();
	worker.use(
		http.post('/api/llm-gateway/admin/providers', async ({ request }) => {
			const body = (await request.json()) as { provider: string };
			created(body);
			return HttpResponse.json({ name: body.provider });
		}),
		http.post('/api/llm-gateway/admin/providers/:name/keys', async ({ params, request }) => {
			keyCreated(params.name, await request.json());
			return keyStatus === 200
				? HttpResponse.json({ id: 'k1' })
				: HttpResponse.json({ error: { message: 'invalid API key' } }, { status: keyStatus });
		}),
		http.delete('/api/llm-gateway/admin/providers/:name', ({ params }) => {
			deleted(params.name);
			return new HttpResponse(null, { status: 204 });
		})
	);
	return { created, keyCreated, deleted };
}

async function openDialog(configured: string[] = [], existingKeyNames: string[] = []) {
	const onCreated = vi.fn();
	const dialog = await renderOpenDialog(AddProviderDialog, {
		configured,
		existingKeyNames,
		onCreated
	});
	return { dialog, onCreated };
}

describe('AddProviderDialog', () => {
	it('hides providers that are already configured and filters by search', async () => {
		const { dialog } = await openDialog(['openai']);

		await expect.element(dialog.getByRole('button', { name: /^Anthropic/ })).toBeInTheDocument();
		await expect.element(dialog.getByRole('button', { name: /^OpenAI / })).not.toBeInTheDocument();

		await dialog.getByPlaceholder('Search providers').fill('groq');
		await expect.element(dialog.getByRole('button', { name: /^Groq/ })).toBeInTheDocument();
		await expect
			.element(dialog.getByRole('button', { name: /^Anthropic/ }))
			.not.toBeInTheDocument();
		// A custom provider is always offered.
		await expect
			.element(dialog.getByRole('button', { name: /Custom provider/ }))
			.toBeInTheDocument();
	});

	it('creates a catalog provider and its first key with the next free key name', async () => {
		const { created, keyCreated } = mockCreate();
		const { dialog, onCreated } = await openDialog([], ['anthropic-key-1']);

		await dialog.getByRole('button', { name: /^Anthropic/ }).click();
		await expect.element(page.getByRole('heading', { name: 'Add Anthropic' })).toBeInTheDocument();
		const submit = dialog.getByRole('button', { name: 'Add Provider' });
		await expect.element(submit).toBeDisabled();
		await dialog.getByPlaceholder('sk-ant-...').fill('sk-ant-secret');
		await submit.click();

		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledWith('anthropic'));
		expect(created).toHaveBeenCalledWith({
			provider: 'anthropic',
			network_config: {
				default_request_timeout_in_seconds: 300,
				max_retries: 2,
				retry_backoff_initial: 500,
				retry_backoff_max: 5000
			}
		});
		expect(keyCreated).toHaveBeenCalledWith('anthropic', {
			name: 'anthropic-key-2',
			models: ['*'],
			weight: 1,
			enabled: true,
			value: 'sk-ant-secret'
		});
	});

	it('creates a keyless custom provider without a key', async () => {
		const { created, keyCreated } = mockCreate();
		const { dialog, onCreated } = await openDialog(['taken']);

		await dialog.getByRole('button', { name: /Custom provider/ }).click();
		const submit = dialog.getByRole('button', { name: 'Add Provider' });

		await dialog.getByLabelText('Name').fill('My LLM');
		await dialog.getByPlaceholder('https://llm.internal.example.com/v1').fill('http://10.0.0.5/v1');
		await expect.element(submit).toBeDisabled();
		await dialog.getByLabelText('Name').fill('taken');
		await expect.element(submit).toBeDisabled();

		await dialog.getByLabelText('Name').fill('my-llm');
		await dialog.getByLabelText('API format').selectOptions('anthropic');
		await dialog.getByLabelText('No API key required').click();
		await dialog.getByLabelText(/Allow private network addresses/).click();
		await submit.click();

		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledWith('my-llm'));
		expect(created).toHaveBeenCalledWith({
			provider: 'my-llm',
			network_config: {
				default_request_timeout_in_seconds: 300,
				max_retries: 2,
				retry_backoff_initial: 500,
				retry_backoff_max: 5000,
				base_url: 'http://10.0.0.5/v1',
				allow_private_network: true
			},
			custom_provider_config: { base_provider_type: 'anthropic', is_key_less: true }
		});
		expect(keyCreated).not.toHaveBeenCalled();
	});

	it('removes the new provider and shows the error when its key is rejected', async () => {
		const { created, deleted } = mockCreate({ keyStatus: 400 });
		const { dialog, onCreated } = await openDialog();

		await dialog.getByRole('button', { name: /^Groq/ }).click();
		await dialog.getByPlaceholder('gsk_...').fill('gsk_bad');
		await dialog.getByRole('button', { name: 'Add Provider' }).click();

		await expect.element(dialog.getByText('invalid API key')).toBeInTheDocument();
		expect(created).toHaveBeenCalledOnce();
		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('groq'));
		expect(onCreated).not.toHaveBeenCalled();
	});
});
