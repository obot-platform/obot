import { worker } from '../../tests/mocks/worker';
import ActivatePage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const setupToken = 'a-high-entropy-owner-setup-token-value';

// On success the page leaves for /change-password. A real navigation would tear down the test
// browser, so capture the call and keep the rest of the module intact.
const goto = vi.fn(async () => {});
vi.mock('$app/navigation', async (importOriginal) => ({
	...(await importOriginal<typeof import('$app/navigation')>()),
	goto: () => goto()
}));

function setFragment(fragment: string) {
	window.history.replaceState(null, '', window.location.pathname + fragment);
}

function mockActivate(respond: () => Response) {
	let body: unknown;
	const activate = vi.fn(async ({ request }: { request: Request }) => {
		body = await request.json();
		return respond();
	});
	worker.use(http.post('/api/local-auth/activate', activate));
	return { activate, sentBody: () => body };
}

afterEach(() => {
	goto.mockClear();
	setFragment('');
});

describe('owner activation page', () => {
	it('exchanges the token from the URL fragment and leaves for the password page', async () => {
		const { activate, sentBody } = mockActivate(() => HttpResponse.json({ activated: true }));
		setFragment(`#token=${setupToken}`);

		render(ActivatePage);

		// Only the token is sent: the endpoint must not accept a caller-supplied email or user ID.
		// Wait on the body: the spy records the call before its request body has finished parsing.
		await vi.waitFor(() => expect(sentBody()).toEqual({ setupToken }));
		expect(activate).toHaveBeenCalledTimes(1);
		await vi.waitFor(() => expect(goto).toHaveBeenCalledTimes(1));
	});

	it('sends a token containing "+" literally rather than form-decoding it to a space', async () => {
		// Operators supply this token, so it can use a standard Base64 alphabet.
		const plusToken = 'aGVsbG8+d29ybGQ/dGhpcytpcythK3Rva2VuK3ZhbHVlKys=';
		const { sentBody } = mockActivate(() => HttpResponse.json({ activated: true }));
		setFragment(`#token=${plusToken}`);

		render(ActivatePage);

		await vi.waitFor(() => expect(sentBody()).toEqual({ setupToken: plusToken }));
	});

	it('strips the setup token from the address bar before anything else can read it', async () => {
		mockActivate(() => HttpResponse.json({ activated: true }));
		setFragment(`#token=${setupToken}`);

		render(ActivatePage);

		// The fragment is the only browser-side copy of a bearer credential, so it must not survive
		// in the URL or in history.
		await vi.waitFor(() => expect(window.location.hash).toBe(''));
		expect(window.location.href).not.toContain(setupToken);
	});

	it('reports an incomplete link without calling the server', async () => {
		const { activate } = mockActivate(() => HttpResponse.json({ activated: true }));
		setFragment('#nottoken=value');

		render(ActivatePage);

		await expect.element(page.getByText('This setup link is incomplete.')).toBeVisible();
		expect(activate).not.toHaveBeenCalled();
	});

	it('treats a malformed percent escape as a broken link instead of throwing', async () => {
		const { activate } = mockActivate(() => HttpResponse.json({ activated: true }));
		// A bare '%' is not a valid escape, so decoding it throws. That must surface as the normal
		// error rather than leaving the page stuck on its loading state with the token still in
		// the address bar.
		setFragment('#token=%');

		render(ActivatePage);

		await expect.element(page.getByText('This setup link is incomplete.')).toBeVisible();
		expect(activate).not.toHaveBeenCalled();
		expect(window.location.hash).toBe('');
	});

	it('shows a rejected link inline instead of redirecting away', async () => {
		mockActivate(() => new HttpResponse('invalid or expired setup link', { status: 401 }));
		setFragment(`#token=${setupToken}`);

		render(ActivatePage);

		// A 401 here must not trigger the app's usual bounce to the provider list, which would
		// discard the token the user still needs.
		await expect
			.element(
				page.getByText(
					'Ask the person who provisioned this environment to reissue the owner setup link.'
				)
			)
			.toBeVisible();
		expect(goto).not.toHaveBeenCalled();
	});
});
