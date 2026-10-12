import { doPost } from './http';
import { importOpenAPI } from './openapi';
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
