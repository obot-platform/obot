import CommunitySignUpForm from './CommunitySignUpForm.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('CommunitySignUpForm.svelte', () => {
	it('renders header copy and fields by default', async () => {
		await render(CommunitySignUpForm, {
			signUpMessage: 'Register to unlock remaining providers.'
		});

		await expect
			.element(page.getByRole('heading', { name: 'Get Access Now!', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText('Register to unlock remaining providers.', { exact: true }))
			.toBeVisible();
		await expect.element(page.getByLabelText('Name', { exact: true })).toBeVisible();
		await expect.element(page.getByLabelText('Email', { exact: true })).toBeVisible();
		await expect.element(page.getByLabelText('Company', { exact: false })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Register', exact: true })).toBeVisible();
	});

	it('hides the header when showHeader is false', async () => {
		await render(CommunitySignUpForm, {
			showHeader: false,
			signUpMessage: 'Should not appear in the form header'
		});

		await expect
			.element(page.getByRole('heading', { name: 'Get Access Now!', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText('Should not appear in the form header', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('submits trimmed enrollment data to the endpoint', async () => {
		const endpoint = vi.fn().mockResolvedValue({ licenseKey: 'community-key' });
		const onSubmit = vi.fn();

		await render(CommunitySignUpForm, { endpoint, onSubmit });

		await page.getByLabelText('Name', { exact: true }).fill('  Ada Lovelace  ');
		await page.getByLabelText('Email', { exact: true }).fill('  ada@example.com  ');
		await page.getByLabelText('Company', { exact: false }).fill('  Analytical Engine  ');
		await page.getByRole('button', { name: 'Register', exact: true }).click();

		await vi.waitFor(() => {
			expect(endpoint).toHaveBeenCalledWith({
				name: 'Ada Lovelace',
				email: 'ada@example.com',
				company: 'Analytical Engine'
			});
			expect(onSubmit).toHaveBeenCalledWith({ licenseKey: 'community-key' });
		});
	});

	it('shows an error when registration fails', async () => {
		const endpoint = vi.fn().mockRejectedValue(new Error('Mailbox full'));

		await render(CommunitySignUpForm, { endpoint });

		await page.getByLabelText('Name', { exact: true }).fill('Ada Lovelace');
		await page.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByRole('button', { name: 'Register', exact: true }).click();

		await expect.element(page.getByText('Mailbox full', { exact: true })).toBeVisible();
	});
});
