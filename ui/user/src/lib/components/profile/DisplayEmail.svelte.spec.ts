import DisplayEmail from './DisplayEmail.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('email presentation', () => {
	it('lowercases a username and appends the configured domain without mutating it', async () => {
		const user = { email: 'JDOE', emailDisplayDomain: 'Example.COM' };
		await render(DisplayEmail, { user });
		await expect.element(page.getByText('jdoe@example.com', { exact: true })).toBeVisible();
		expect(user.email).toBe('JDOE');
	});

	it('does not append a second domain to a full email address', async () => {
		await render(DisplayEmail, {
			user: { email: 'JDOE@Other.COM', emailDisplayDomain: 'example.com' }
		});
		await expect.element(page.getByText('jdoe@other.com', { exact: true })).toBeVisible();
	});

	it('preserves existing email display when no domain is configured', async () => {
		await render(DisplayEmail, { user: { email: 'JDOE' } });
		await expect.element(page.getByText('JDOE', { exact: true })).toBeVisible();
	});

	it('accepts a domain with an initial @ and updates when it is cleared', async () => {
		const screen = await render(DisplayEmail, {
			user: { email: 'JDOE', emailDisplayDomain: '@Example.COM' }
		});
		await expect.element(page.getByText('jdoe@example.com', { exact: true })).toBeVisible();
		await screen.rerender({ user: { email: 'JDOE', emailDisplayDomain: '' } });
		await expect.element(page.getByText('JDOE', { exact: true })).toBeVisible();
	});
});
