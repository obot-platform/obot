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
		http.post('/api/openapi/import', async ({ request }) => {
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
