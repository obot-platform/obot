import { doPost } from './http';
import { importOpenAPI, validateOpenAPIDestination } from './openapi';
import { expect, it, vi } from 'vitest';

vi.mock('./http', () => ({ doPost: vi.fn() }));

it('trims the schema URL before importing without mutating the caller', async () => {
	vi.mocked(doPost).mockResolvedValue({ suggestedHeaders: [] });
	const config = { source: { url: ' \nhttps://example.com/schema.json\t ' } };
	await importOpenAPI(config);
	expect(doPost).toHaveBeenCalledWith(
		'/openapi-import',
		{ source: { url: 'https://example.com/schema.json' } },
		{ dontLogErrors: true }
	);
	expect(config.source.url).toBe(' \nhttps://example.com/schema.json\t ');
});

it('trims both validation URLs and preserves uploaded content', async () => {
	vi.mocked(doPost).mockResolvedValue({ destinationValidation: { valid: true } });
	const content = '\nopenapi: 3.1.0\n  # Keep indentation and whitespace\n';
	for (const source of [{ url: ' \thttps://example.com/schema.json\n ' }, { content }]) {
		await validateOpenAPIDestination({
			source,
			baseURL: ' \nhttps://api.example.com/v1\t '
		});
		expect(doPost).toHaveBeenLastCalledWith(
			'/openapi-import',
			{
				source: source.url ? { url: 'https://example.com/schema.json' } : { content },
				baseURL: 'https://api.example.com/v1'
			},
			{ dontLogErrors: true }
		);
	}
});

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
