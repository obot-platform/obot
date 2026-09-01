import { worker } from '../../tests/mocks/worker';
import ChangePasswordPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// A successful change ends in window.location.assign, which would navigate the test browser away.
// These cases deliberately cover the paths that stop short of that: client-side validation and a
// server rejection. Both still prove what is sent and what the user is shown.
function mockChangePassword(respond: () => Response) {
	let body: unknown;
	const changePassword = vi.fn(async ({ request }: { request: Request }) => {
		body = await request.json();
		return respond();
	});
	worker.use(http.post('/api/local-auth/change-password', changePassword));
	return { changePassword, sentBody: () => body };
}

async function fillPasswords(newPassword: string, confirmation: string) {
	await page.getByLabelText(/New password/).fill(newPassword);
	await page.getByLabelText(/Confirm password/).fill(confirmation);
	await page.getByRole('button', { name: 'Set password and continue' }).click();
}

describe('forced password change page', () => {
	it('rejects a mismatched confirmation without contacting the server', async () => {
		const { changePassword } = mockChangePassword(() => HttpResponse.json({ changed: true }));

		render(ChangePasswordPage);
		await fillPasswords('a-long-enough-password', 'a-different-password');

		await expect.element(page.getByText('The passwords do not match.')).toBeVisible();
		expect(changePassword).not.toHaveBeenCalled();
	});

	it('sends only the new password and surfaces a server rejection inline', async () => {
		const { changePassword, sentBody } = mockChangePassword(
			() => new HttpResponse('password setup has already been completed', { status: 409 })
		);

		render(ChangePasswordPage);
		await fillPasswords('a-long-enough-password', 'a-long-enough-password');

		// The endpoint derives the user from the session, so the page must not be able to name an
		// email, user ID, or role. Wait on the body: the spy records the call before its request
		// body has finished parsing.
		await vi.waitFor(() => expect(sentBody()).toEqual({ password: 'a-long-enough-password' }));
		expect(changePassword).toHaveBeenCalledTimes(1);
		await expect.element(page.getByText(/password setup has already been completed/)).toBeVisible();
	});

	it('offers a way to leave without completing setup', async () => {
		render(ChangePasswordPage);

		// "Finish later" signs out rather than consuming the setup link, so an interrupted setup can
		// be resumed from the original email.
		const finishLater = page.getByRole('link', { name: 'Finish later' });
		await expect.element(finishLater).toBeVisible();
		await expect.element(finishLater).toHaveAttribute('href', '/oauth2/sign_out?rd=/');
	});
});
