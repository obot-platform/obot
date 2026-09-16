import LoginPage from './+page.svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const emailKey = 'local-auth-email';
const email = 'someone@example.com';

// The form posts to the auth provider, which redirects back here with `?error=` on failure.
function setQuery(query: string) {
	window.history.replaceState(null, '', window.location.pathname + query);
}

afterEach(() => setQuery(''));

describe('local login page', () => {
	it('preserves the redirect destination in the login form', async () => {
		const rd = '/mcp-servers?view=all&search=a%20b#details';
		setQuery('?rd=' + encodeURIComponent(rd));

		render(LoginPage);

		await expect.element(page.getByCSS('input[name="rd"]')).toHaveValue(rd);
	});

	it('restores the saved email after a failed login and clears it', async () => {
		sessionStorage.setItem(emailKey, email);
		setQuery('?error=Incorrect+email+or+password.');

		render(LoginPage);

		await expect.element(page.getByText('Incorrect email or password.')).toBeVisible();
		await expect.element(page.getByLabelText('Email')).toHaveValue(email);
		await expect.element(page.getByLabelText('Password')).toHaveValue('');
		expect(sessionStorage.getItem(emailKey)).toBeNull();
	});

	it('does not reuse a saved email on a visit without an error', async () => {
		sessionStorage.setItem(emailKey, email);

		render(LoginPage);

		await expect.element(page.getByLabelText('Email')).toHaveValue('');
		expect(sessionStorage.getItem(emailKey)).toBeNull();
	});

	it('saves the email when the form is submitted', async () => {
		render(LoginPage);
		await page.getByLabelText('Email').fill(email);

		// A real submit navigates the test browser away. Dispatching the event on the form runs the
		// page's handler without submitting anything.
		page
			.getByRole('button', { name: 'Sign in' })
			.element()
			.closest('form')
			?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

		expect(sessionStorage.getItem(emailKey)).toBe(email);
	});
});
