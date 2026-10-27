import { replaceState } from '$app/navigation';
import { TesterOAuth } from './tester-oauth';
import { UnauthorizedError } from '@modelcontextprotocol/sdk/client/auth.js';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('$app/navigation', () => ({ replaceState: vi.fn() }));

const endpoint = new URL('https://obot.test/mcp-connect/vmcp1');
const returnURL = 'https://obot.test/mcp-servers?inspector=vmcp1';

beforeEach(() => {
	const entries = new Map<string, string>();
	vi.stubGlobal('sessionStorage', {
		getItem: (key: string) => entries.get(key) ?? null,
		setItem: (key: string, value: string) => entries.set(key, value),
		removeItem: (key: string) => entries.delete(key)
	});
	vi.stubGlobal('window', { location: { href: returnURL } });
	vi.clearAllMocks();
});
afterEach(() => vi.unstubAllGlobals());

function oauthFetch() {
	return vi.fn<typeof fetch>(async (input, init) => {
		const url = new URL(input instanceof Request ? input.url : String(input));
		if (url.pathname.startsWith('/.well-known/oauth-protected-resource')) {
			return Response.json({
				resource: endpoint.href,
				authorization_servers: ['https://obot.test']
			});
		}
		if (url.pathname === '/.well-known/oauth-authorization-server') {
			return Response.json({
				issuer: 'https://obot.test',
				authorization_endpoint: 'https://obot.test/authorize',
				token_endpoint: 'https://obot.test/token',
				registration_endpoint: 'https://obot.test/register',
				response_types_supported: ['code'],
				code_challenge_methods_supported: ['S256'],
				token_endpoint_auth_methods_supported: ['none']
			});
		}
		if (url.pathname === '/register') {
			return Response.json(
				{ ...JSON.parse(String(init?.body)), client_id: 'tester' },
				{ status: 201 }
			);
		}
		if (url.pathname === '/token') {
			return Response.json({ access_token: 'not-retained', token_type: 'Bearer' });
		}
		if (url.pathname === endpoint.pathname) {
			return new Response(null, {
				status: 403,
				headers: {
					'WWW-Authenticate':
						'Bearer error="insufficient_scope", scope="obot:upstream:ticket", resource_metadata="https://obot.test/.well-known/oauth-protected-resource/mcp-connect/vmcp1"'
				}
			});
		}
		throw new Error(`Unexpected OAuth fetch: ${url.pathname}`);
	});
}

describe('tester OAuth challenges', () => {
	it('carries captured scope through PKCE and restores the tester without retaining tokens', async () => {
		const prompted = vi.fn();
		const provider = new TesterOAuth(endpoint, prompted);
		const fetcher = oauthFetch();
		await expect(provider.fetch(fetcher)(endpoint)).rejects.toBeInstanceOf(UnauthorizedError);

		const authorization = new URL(prompted.mock.calls[0][0]);
		expect(authorization.searchParams.get('scope')).toBe('obot:upstream:ticket');
		expect(authorization.searchParams.get('code_challenge_method')).toBe('S256');
		expect(fetcher.mock.calls.filter(([input]) => String(input) === endpoint.href)).toHaveLength(1);

		const callback = new URL(provider.redirectUrl);
		callback.searchParams.set('code', 'code1');
		callback.searchParams.set('state', provider.state());
		window.location.href = callback.href;
		const restored = new TesterOAuth(endpoint, prompted);
		const verifier = restored.codeVerifier();
		await restored.complete(fetcher);

		const exchange = fetcher.mock.calls.find(
			([input]) => String(input) === 'https://obot.test/token'
		);
		const form = new URLSearchParams(String(exchange?.[1]?.body));
		expect(form.get('code_verifier')).toBe(verifier);
		expect(replaceState).toHaveBeenCalledWith(new URL(returnURL), {});
		expect(restored.tokens()).toBeUndefined();
		expect(sessionStorage.getItem(`mcp-tester-oauth:${endpoint.href}`)).toBeNull();
	});

	it('rejects a mismatched callback state before exchanging a code', async () => {
		const provider = new TesterOAuth(endpoint, vi.fn());
		const callback = new URL(provider.redirectUrl);
		callback.searchParams.set('code', 'code1');
		callback.searchParams.set('state', 'wrong');
		window.location.href = callback.href;
		const fetcher = oauthFetch();

		await expect(provider.complete(fetcher)).rejects.toThrow('Invalid tester OAuth state');
		expect(fetcher).not.toHaveBeenCalled();
	});

	it('shares one authorization flow across simultaneous challenges', async () => {
		const prompted = vi.fn();
		const provider = new TesterOAuth(endpoint, prompted);
		const fetcher = provider.fetch(oauthFetch());
		await Promise.allSettled([fetcher(endpoint), fetcher(endpoint)]);
		expect(prompted).toHaveBeenCalledTimes(1);
	});

	it('uses a fresh ticket after an earlier authorization flow settles', async () => {
		const prompted = vi.fn();
		const provider = new TesterOAuth(endpoint, prompted);
		const fetcher = oauthFetch();
		const wrapped = provider.fetch(fetcher);
		await expect(wrapped(endpoint)).rejects.toBeInstanceOf(UnauthorizedError);

		fetcher.mockResolvedValueOnce(
			new Response(null, {
				status: 403,
				headers: {
					'WWW-Authenticate': 'Bearer error="insufficient_scope", scope="obot:upstream:fresh"'
				}
			})
		);
		await expect(wrapped(endpoint)).rejects.toBeInstanceOf(UnauthorizedError);

		expect(prompted).toHaveBeenCalledTimes(2);
		const authorization = new URL(prompted.mock.calls[1][0]);
		expect(authorization.searchParams.get('scope')).toBe('obot:upstream:fresh');
	});

	it.each([401, 403])('preserves ordinary %s responses without starting OAuth', async (status) => {
		const prompted = vi.fn();
		const provider = new TesterOAuth(endpoint, prompted);
		const response = new Response('denied', { status });
		expect(await provider.fetch(vi.fn(async () => response))(endpoint)).toBe(response);
		if (status === 401) {
			expect(prompted).toHaveBeenCalledWith();
		} else {
			expect(prompted).not.toHaveBeenCalled();
		}
	});
});
