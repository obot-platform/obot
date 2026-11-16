import { load as loadApiKey } from './+page';
import { load as loadApiKeyUsage } from './[api_key]/+page';
import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';

// The user's API keys were the "Agents" tab, so bookmarks to its pages still have to work.
function redirectOf(fn: () => unknown) {
	try {
		fn();
	} catch (err) {
		if (isRedirect(err)) return err;
		throw err;
	}
	throw new Error('expected a redirect');
}

describe('old /identity-access/agents URLs', () => {
	it('redirects an API key to its page under api-keys', () => {
		const redirect = redirectOf(() =>
			loadApiKey({
				params: { id: '42' },
				url: new URL('http://localhost/identity-access/agents/42?x=1')
			} as unknown as Parameters<typeof loadApiKey>[0])
		);

		expect(redirect.status).toBe(301);
		expect(redirect.location).toBe('/identity-access/api-keys/42?x=1');
	});

	it('redirects an API key usage page to its page under api-keys', () => {
		const prefix = 'ok1-7-42-*****';
		const redirect = redirectOf(() =>
			loadApiKeyUsage({
				params: { id: '42', api_key: prefix },
				url: new URL('http://localhost/x?tab=llm')
			} as unknown as Parameters<typeof loadApiKeyUsage>[0])
		);

		expect(redirect.status).toBe(301);
		expect(redirect.location).toBe(
			`/identity-access/api-keys/42/${encodeURIComponent(prefix)}?tab=llm`
		);
	});
});
