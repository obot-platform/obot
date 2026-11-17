import { SCIM_VIEW_PATH } from './constants';
import { createHttpError, handleRouteError, HttpError, parseErrorReasons } from './errors';
import type { Profile } from './services';
import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';

// The page that handleRouteError sends the browser to, from the redirect it throws.
function redirectedTo(err: HttpError, profile?: Profile): URL {
	let redirect: unknown;
	try {
		handleRouteError(err, SCIM_VIEW_PATH, profile);
	} catch (thrown) {
		redirect = thrown;
	}
	expect(isRedirect(redirect)).toBe(true);
	return new URL((redirect as { location: string }).location, 'http://localhost');
}

describe('handleRouteError', () => {
	it('returns a signed-out user to the page after signing in, query included', () => {
		const location = redirectedTo(new HttpError(401, 'unauthorized'));

		expect(location.pathname).toBe('/');
		expect(location.searchParams.get('rd')).toBe(SCIM_VIEW_PATH);
	});

	it('returns a user without a role to the page, query included', () => {
		const location = redirectedTo(new HttpError(403, 'forbidden'), { role: 0 } as Profile);

		expect(location.pathname).toBe('/');
		expect(location.searchParams.get('rd')).toBe(SCIM_VIEW_PATH);
	});
});

describe('parseErrorReasons', () => {
	it('reads the reasons of a JSON error body', () => {
		const body = JSON.stringify({
			error: 'configure the profile first',
			reasons: [{ componentID: 'github', code: 'configuration_required', field: '__url' }]
		});
		expect(
			parseErrorReasons(createHttpError(400, '/vmcps/v/profiles/p/components/c/oauth', body))
		).toEqual([{ componentID: 'github', code: 'configuration_required', field: '__url' }]);
	});

	it('finds none in a plain text body, an empty list, or something that is not an HTTP error', () => {
		expect(parseErrorReasons(createHttpError(400, '/x', 'plain text'))).toEqual([]);
		expect(parseErrorReasons(createHttpError(400, '/x', '{"error":"x","reasons":[]}'))).toEqual([]);
		expect(
			parseErrorReasons(createHttpError(400, '/x', '{"error":"x","reasons":[1,{"nocode":true}]}'))
		).toEqual([]);
		expect(parseErrorReasons(new Error('400 /x: {"reasons":[{"code":"a"}]}'))).toEqual([]);
		expect(parseErrorReasons(undefined)).toEqual([]);
	});
});
