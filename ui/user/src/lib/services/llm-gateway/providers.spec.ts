import {
	authenticationFields,
	savedAuthenticationMethod,
	catalogEntry,
	providerIcon,
	providerLabel
} from './providers';
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

describe('imported Mantle authentication', () => {
	it('uses Mantle field paths for both authentication methods', () => {
		const entry = catalogEntry('bedrock_mantle')!;
		expect(authenticationFields(entry, 'api-key').map((field) => field.path)).toEqual([
			'value',
			'bedrock_mantle_key_config.region'
		]);
		expect(authenticationFields(entry, 'credentials').map((field) => field.path)).not.toContain(
			'value'
		);
		expect(
			savedAuthenticationMethod(entry, {
				'bedrock_mantle_key_config.access_key': 'access',
				'bedrock_mantle_key_config.secret_key': 'secret'
			})
		).toBe('credentials');
	});
});
