import { doPost } from './http';
import { importOpenAPI, validateOpenAPIDestination } from './openapi';
import { expect, it, vi } from 'vitest';

vi.mock('./http', () => ({ doPost: vi.fn() }));

it('uses the import summary without a schema snapshot and sends only the source', async () => {
	const result = {
		baseURL: 'https://api.example.com',
		destinationValidation: { valid: true },
		suggestedHeaders: [],
		suggestedMetadata: { name: 'Example' }
	};
	vi.mocked(doPost).mockResolvedValue(result);
	expect(
		await importOpenAPI({
			source: { url: 'https://example.com/schema.json' },
			baseURL: 'https://override.example.com'
		})
	).toEqual(result);
	expect(doPost).toHaveBeenCalledWith(
		'/openapi-import',
		{ source: { url: 'https://example.com/schema.json' } },
		{ dontLogErrors: true }
	);
});

it('validates an override against the import endpoint without sending credentials', async () => {
	vi.mocked(doPost).mockResolvedValue({
		baseURL: 'https://override.example.com/',
		destinationValidation: { valid: false, error: 'API destination is blocked' },
		suggestedHeaders: [],
		suggestedMetadata: {}
	});
	expect(
		await validateOpenAPIDestination({
			source: { url: 'https://example.com/schema.json' },
			baseURL: 'https://override.example.com/'
		})
	).toEqual({ valid: false, error: 'API destination is blocked' });
	expect(doPost).toHaveBeenCalledWith(
		'/openapi-import',
		{
			source: { url: 'https://example.com/schema.json' },
			baseURL: 'https://override.example.com/'
		},
		{ dontLogErrors: true }
	);
});

it('forwards the validation abort signal to the HTTP request', async () => {
	const controller = new AbortController();
	vi.mocked(doPost).mockResolvedValue({ destinationValidation: { valid: true } });
	await validateOpenAPIDestination(
		{ source: { url: 'https://example.com/schema.json' }, baseURL: 'https://api.example.com' },
		controller.signal
	);
	expect(doPost).toHaveBeenCalledWith(
		'/openapi-import',
		{ source: { url: 'https://example.com/schema.json' }, baseURL: 'https://api.example.com' },
		{ dontLogErrors: true, signal: controller.signal }
	);
});

it('normalizes null header suggestions without changing the other import fields', async () => {
	const result = {
		baseURL: 'https://api.example.com',
		destinationValidation: { valid: true },
		suggestedHeaders: null,
		suggestedMetadata: { name: 'Example', description: 'Public API' }
	};
	vi.mocked(doPost).mockResolvedValue(result);
	expect(await importOpenAPI({ source: { url: 'https://example.com/schema.json' } })).toEqual({
		...result,
		suggestedHeaders: []
	});
});
