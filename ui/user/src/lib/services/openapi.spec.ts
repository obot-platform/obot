import { openAPIBaseURL } from './openapi';
import { describe, expect, it } from 'vitest';

describe('OpenAPI base URL placeholder for saved snapshots', () => {
	it('uses the first usable absolute URL without storing an override', () => {
		expect(
			openAPIBaseURL({
				servers: [
					{ url: '/relative' },
					{ url: 'https://{host}/api' },
					{ url: 'https://api.example.com/v1' }
				]
			})
		).toBe('https://api.example.com/v1');
	});
	it('returns no placeholder if there is no usable schema server', () => {
		expect(openAPIBaseURL()).toBe('');
		expect(openAPIBaseURL({ servers: [{ url: '/relative' }] })).toBe('');
	});
});
