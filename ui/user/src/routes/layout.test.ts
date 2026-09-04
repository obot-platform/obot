import { Group } from '$lib/services';
import { load } from './+layout';
import { describe, expect, it, vi } from 'vitest';

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

async function loadWith(fetch: ReturnType<typeof createFetch>) {
	return (await load({ fetch } as unknown as Parameters<NonNullable<typeof load>>[0])) as Exclude<
		Awaited<ReturnType<NonNullable<typeof load>>>,
		void
	>;
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
