import { catalogEntry, providerIcon, providerLabel } from './providers';
import { describe, expect, it } from 'vitest';

describe('imported provider catalog entries', () => {
	it('recognizes Bedrock Mantle while retaining its actual credential shape', () => {
		expect(providerLabel('bedrock_mantle')).toBe('AWS Bedrock');
		expect(providerIcon('bedrock_mantle')).toBe('/admin/assets/amazon-bedrock.png');
		const entry = catalogEntry('bedrock_mantle');
		expect(entry?.id).toBe('bedrock_mantle');
		expect(entry?.keyFields.map((f) => f.path)).toContain('bedrock_mantle_key_config.region');
		expect(entry?.keyFields.some((f) => f.path.startsWith('bedrock_key_config.'))).toBe(false);
	});
});
