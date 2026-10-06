import LanguageSelect from './LanguageSelect.svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const { setLocale } = vi.hoisted(() => ({ setLocale: vi.fn() }));

vi.mock('$lib/paraglide/runtime', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/paraglide/runtime')>()),
	// The real implementation reloads the page, which would tear down the test.
	setLocale
}));

describe('LanguageSelect.svelte', () => {
	beforeEach(() => {
		setLocale.mockClear();
	});

	it('lists every supported language by its native name, defaulting to English', async () => {
		render(LanguageSelect);

		await expect.element(page.getByRole('button', { name: /Language/ })).toMatchTextContent('EN');
		await page.getByRole('button', { name: /Language/ }).click();

		await expect.element(page.getByRole('heading', { name: 'Select Language' })).toBeVisible();
		await expect.element(page.getByRole('radio', { name: 'English' })).toBeChecked();
		for (const name of ['English', '日本語', '한국어', '简体中文']) {
			await expect.element(page.getByRole('radio', { name })).toBeInTheDocument();
		}
	});

	it('switches the locale when a different language is applied', async () => {
		render(LanguageSelect);

		await page.getByRole('button', { name: /Language/ }).click();
		await page.getByRole('radio', { name: '日本語' }).click();
		await page.getByRole('button', { name: 'Apply' }).click();

		expect(setLocale).toHaveBeenCalledWith('ja');
	});

	it('keeps the current locale when the dialog is cancelled', async () => {
		render(LanguageSelect);

		await page.getByRole('button', { name: /Language/ }).click();
		await page.getByRole('radio', { name: '한국어' }).click();
		await page.getByRole('button', { name: 'Cancel' }).click();

		expect(setLocale).not.toHaveBeenCalled();
	});

	it('renders in the locale saved from a previous visit', async () => {
		localStorage.setItem('PARAGLIDE_LOCALE', 'ja');
		render(LanguageSelect);

		await page.getByRole('button', { name: /言語/ }).click();

		await expect.element(page.getByRole('heading', { name: '言語を選択' })).toBeVisible();
		await expect.element(page.getByRole('radio', { name: '日本語' })).toBeChecked();
	});
});
