import { worker } from '../../../tests/mocks/worker';
import OpenAPIConfiguration from './OpenAPIConfiguration.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const result = {
	schema: {
		openapi: '3.1.0',
		info: { title: 'Example', version: '1' },
		servers: [{ url: 'https://api.example.com' }],
		paths: {}
	},
	baseURL: 'https://api.example.com',
	suggestedHeaders: [],
	suggestedMetadata: {}
};

it('imports the schema when Enter is pressed in the schema URL', async () => {
	const imported = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			imported(await request.json());
			return HttpResponse.json(result);
		})
	);
	const onComplete = vi.fn();
	await render(OpenAPIConfiguration, {
		config: { source: {} },
		current: {},
		onComplete
	});
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/schema.json');
	await userEvent.keyboard('{Enter}');
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	expect(imported).toHaveBeenCalledWith(
		expect.objectContaining({ source: { url: 'https://example.com/schema.json' } })
	);
});

it('keeps existing metadata when re-importing a schema', async () => {
	worker.use(
		http.post('/api/openapi-import', () =>
			HttpResponse.json({
				...result,
				suggestedMetadata: {
					name: 'New name',
					description: 'New description',
					shortDescription: 'New summary',
					icon: 'https://example.com/new.png'
				}
			})
		)
	);
	const onComplete = vi.fn();
	await render(OpenAPIConfiguration, {
		config: { source: { url: 'https://example.com/schema.json' }, schema: result.schema },
		current: {
			name: 'Existing name',
			description: 'Existing description',
			shortDescription: 'Existing summary',
			icon: 'https://example.com/existing.png'
		},
		onComplete
	});
	await page.getByRole('button', { name: 'Replace schema' }).click();
	await page.getByRole('button', { name: 'Import schema' }).click();
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	expect(onComplete).toHaveBeenCalledWith(
		expect.objectContaining({ schema: result.schema }),
		expect.objectContaining({ schema: result.schema }),
		{}
	);
	expect(document.querySelector('dialog')?.open).toBe(false);
});

it('imports a replacement without sending the configured override', async () => {
	const imported = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			imported(await request.json());
			return HttpResponse.json({ ...result, destinationValidation: { valid: true } });
		})
	);
	const onComplete = vi.fn();
	await render(OpenAPIConfiguration, {
		config: {
			source: { url: 'https://example.com/schema.json' },
			schema: result.schema,
			baseURL: 'https://override.example.com'
		},
		current: {},
		onComplete
	});
	await page.getByRole('button', { name: 'Replace schema' }).click();
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Import schema' }).click();
	await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
	expect(imported).toHaveBeenCalledWith({ source: { url: 'https://example.com/schema.json' } });
	expect(onComplete.mock.calls[0][0].baseURL).toBe('https://override.example.com');
});
