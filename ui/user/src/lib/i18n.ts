import { getLocale, locales, setLocale, type Locale } from '$lib/paraglide/runtime';

export { m } from '$lib/paraglide/messages';
export { getLocale, locales, type Locale };

// Names are shown in their own language so users can find theirs regardless of the current locale.
export const LOCALE_NAMES: Record<Locale, string> = {
	en: 'English',
	ja: '日本語',
	ko: '한국어',
	'zh-CN': '简体中文'
};

export const LOCALE_SHORTHAND: Record<Locale, string> = {
	en: 'EN',
	ja: 'JA',
	ko: 'KO',
	'zh-CN': 'ZH-CN'
};

/** Persists the locale and reloads the page so every message re-renders in the new language. */
export function changeLocale(locale: Locale) {
	setLocale(locale);
}

/** Mirrors the active locale onto `<html lang>` for screen readers, fonts, and line breaking. */
export function applyDocumentLocale() {
	if (typeof document === 'undefined') return;
	document.documentElement.lang = getLocale();
}
