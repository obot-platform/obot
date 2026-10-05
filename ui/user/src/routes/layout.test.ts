import { Group } from '$lib/services';
import { load } from './+layout';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function response(body: unknown, status = 200) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function createFetch(groups: string[], telemetryStatus = 200) {
	return vi.fn(async (input: RequestInfo | URL) => {
		const path = new URL(String(input)).pathname;
		switch (path) {
			case '/api/version':
				return response({});
			case '/api/license':
				return response({});
			case '/api/app-preferences':
				return response({});
			case '/api/me':
				return response({
					id: 'user-1',
					username: 'admin@example.com',
					email: 'admin@example.com',
					iconURL: '',
					role: 1,
					effectiveRole: 1,
					groups
				});
			case '/api/default-model-aliases':
			case '/api/models':
				return response({ items: [] });
			case '/api/app-notification':
				return response({});
			case '/api/product-telemetry-consent':
				return telemetryStatus === 200
					? response({})
					: response({ error: 'unavailable' }, telemetryStatus);
			default:
				throw new Error(`Unexpected request: ${path}`);
		}
	});
}

async function loadWith(fetch: ReturnType<typeof createFetch>, url = new URL('http://localhost/')) {
	return (await load({ fetch, url } as unknown as Parameters<
		NonNullable<typeof load>
	>[0])) as Exclude<Awaited<ReturnType<NonNullable<typeof load>>>, void>;
}

describe('root layout product analytics consent', () => {
	it.each([
		['auditor', [Group.AUDITOR]],
		['power user', [Group.POWERUSER]],
		['basic user', [Group.USER]]
	])('does not request consent for a %s', async (_name, groups) => {
		const fetch = createFetch(groups);
		const data = await loadWith(fetch);

		expect(
			fetch.mock.calls.some(([input]) => String(input).includes('/api/product-telemetry-consent'))
		).toBe(false);
		expect(data.productTelemetryConsentAvailable).toBeUndefined();
	});

	it.each([
		['Admin', [Group.ADMIN]],
		['Owner', [Group.OWNER, Group.ADMIN]]
	])('loads undecided consent for an %s', async (_name, groups) => {
		const fetch = createFetch(groups);
		const data = await loadWith(fetch);

		expect(
			fetch.mock.calls.filter(([input]) => String(input).includes('/api/product-telemetry-consent'))
		).toHaveLength(1);
		expect(data.productTelemetryConsentAvailable).toBe(true);
		expect(data.productTelemetryConsent).toEqual({});
	});

	it('marks the consent controls unavailable on 404', async () => {
		const data = await loadWith(createFetch([Group.ADMIN], 404));
		expect(data.productTelemetryConsentAvailable).toBe(false);
	});

	it('does not expose consent UI after another read failure', async () => {
		const data = await loadWith(createFetch([Group.ADMIN], 500));
		expect(data.productTelemetryConsentAvailable).toBeUndefined();
	});
});

function createRefusedProfileFetch(status: number, body: string) {
	return vi.fn(async (input: RequestInfo | URL) => {
		const path = new URL(String(input)).pathname;
		switch (path) {
			case '/api/me':
				return new Response(body, { status });
			case '/api/version':
			case '/api/license':
			case '/api/app-preferences':
				return response({});
			default:
				throw new Error(`Unexpected request: ${path}`);
		}
	});
}

describe('root layout account status', () => {
	// cookies holds the browser's cookies for the page, as document.cookie reads them.
	let cookies: Map<string, string>;

	beforeEach(() => {
		const items = new Map<string, string>();
		vi.stubGlobal('sessionStorage', {
			getItem: (key: string) => items.get(key) ?? null,
			setItem: (key: string, value: string) => items.set(key, value),
			removeItem: (key: string) => items.delete(key)
		});
		cookies = new Map();
		vi.stubGlobal('document', {
			get cookie() {
				return [...cookies].map(([name, value]) => `${name}=${value}`).join('; ');
			},
			set cookie(cookie: string) {
				const [pair, ...attributes] = cookie.split(';');
				const [name, value] = pair.split('=');
				if (attributes.some((attribute) => attribute.trim() === 'Max-Age=0')) {
					cookies.delete(name);
				} else {
					cookies.set(name, value);
				}
			}
		});
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('marks an account the server refused as inactive', async () => {
		const data = await loadWith(
			createRefusedProfileFetch(403, 'Your account is not active. Contact your administrator.')
		);

		expect(data.profile.unauthorized).toBe(true);
		expect(data.profile.accountInactive).toBe(true);
	});

	it('marks an account inactive when the server sent its page load to the login page', async () => {
		// The server ended the session as it redirected, so the profile request is merely signed out.
		cookies.set('obot_account_inactive', 'true');
		const data = await loadWith(
			createRefusedProfileFetch(401, 'unauthorized'),
			new URL('http://localhost/?inactive=true')
		);

		expect(data.profile.unauthorized).toBe(true);
		expect(data.profile.accountInactive).toBe(true);
		expect(cookies.has('obot_account_inactive')).toBe(false);
	});

	it('does not trust the login page parameter without the cookie that the server sets', async () => {
		// Anyone can link to the login page with the parameter.
		const data = await loadWith(
			createRefusedProfileFetch(401, 'unauthorized'),
			new URL('http://localhost/?inactive=true')
		);

		expect(data.profile.accountInactive).toBe(false);
	});

	it("ignores the server's cookie once someone is signed in", async () => {
		cookies.set('obot_account_inactive', 'true');
		const data = await loadWith(
			createFetch([Group.USER]),
			new URL('http://localhost/?inactive=true')
		);

		expect(data.profile.accountInactive).toBeUndefined();
		expect(cookies.has('obot_account_inactive')).toBe(false);
	});

	it('does not mark a signed-out visitor as inactive', async () => {
		const data = await loadWith(createRefusedProfileFetch(401, 'unauthorized'));

		expect(data.profile.unauthorized).toBe(true);
		expect(data.profile.accountInactive).toBe(false);
	});

	it('keeps the account inactive across the loads that follow the refusal', async () => {
		await loadWith(
			createRefusedProfileFetch(403, 'Your account is not active. Contact your administrator.')
		);

		// The refusal ended the session, so the load after a redirect is merely signed out.
		const redirected = await loadWith(createRefusedProfileFetch(401, 'unauthorized'));
		expect(redirected.profile.accountInactive).toBe(true);
	});

	it('forgets the inactive account once someone signs in', async () => {
		await loadWith(
			createRefusedProfileFetch(403, 'Your account is not active. Contact your administrator.')
		);
		await loadWith(createFetch([Group.USER]));

		const signedOut = await loadWith(createRefusedProfileFetch(401, 'unauthorized'));
		expect(signedOut.profile.accountInactive).toBe(false);
	});
});
