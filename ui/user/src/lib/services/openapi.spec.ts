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
	it.each([
		{ kind: 'bare query marker', url: 'https://api.example.com?' },
		{ kind: 'backslash', url: 'https://api.example.com\\v1' },
		{ kind: 'surrounding whitespace', url: ' https://api.example.com ' },
		{ kind: 'embedded tab', url: 'https://api.example.com/\tpath' }
	])('skips a server URL with $kind', ({ url }) => {
		expect(openAPIBaseURL({ servers: [{ url }] })).toBe('');
		expect(openAPIBaseURL({ servers: [{ url }, { url: 'https://usable.example.com/v1' }] })).toBe(
			'https://usable.example.com/v1'
		);
	});
});
