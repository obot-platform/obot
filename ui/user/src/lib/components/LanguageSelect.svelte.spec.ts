import LanguageSelect from './LanguageSelect.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const { setLocale } = vi.hoisted(() => ({ setLocale: vi.fn() }));

vi.mock('$lib/paraglide/runtime', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/paraglide/runtime')>()),
	// The real implementation reloads the page, which would tear down the test.
	setLocale
}));

describe('LanguageSelect.svelte', () => {
	it('lists every supported language by its native name, defaulting to English', async () => {
		render(LanguageSelect);

		const select = page.getByRole('combobox', { name: 'Language' });
		await expect.element(select).toHaveValue('en');
		for (const name of ['English', '日本語', '한국어', '简体中文']) {
			await expect.element(page.getByRole('option', { name })).toBeInTheDocument();
		}
	});

	it('switches the locale when a different language is chosen', async () => {
		render(LanguageSelect);

		await page.getByRole('combobox', { name: 'Language' }).selectOptions('ja');

		expect(setLocale).toHaveBeenCalledWith('ja');
	});

	it('renders in the locale saved from a previous visit', async () => {
		localStorage.setItem('PARAGLIDE_LOCALE', 'ja');
		render(LanguageSelect);

		const select = page.getByRole('combobox', { name: '言語' });
		await expect.element(select).toHaveValue('ja');
	});
});
