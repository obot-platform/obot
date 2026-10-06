import { describe, expect, it } from 'vitest';

type Catalog = Record<string, string>;

// Each feature area keeps its own catalog under messages/<area>/{locale}.json; the shell lives at the root.
// Areas follow product routes (mcps, skills, identity_access, platform, ...) and keys are prefixed with that area plus the tab or feature.
const catalogs = import.meta.glob<Catalog>('../../messages/**/*.json', {
	eager: true,
	import: 'default'
});

const byArea = new Map<string, Record<string, Catalog>>();
for (const [path, catalog] of Object.entries(catalogs)) {
	const [, area = '', locale] = path.match(/messages\/(?:(.+)\/)?([^/]+)\.json$/) ?? [];
	const messages = Object.fromEntries(Object.entries(catalog).filter(([key]) => key !== '$schema'));
	byArea.set(area, { ...byArea.get(area), [locale]: messages });
}

const translatedLocales = ['ja', 'ko', 'zh-CN'];
const placeholders = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort();

describe.each([...byArea.entries()])('message catalog "%s"', (_, locales) => {
	const en = locales.en ?? {};

	it.each(translatedLocales)('%s defines exactly the keys in en.json', (locale) => {
		expect(Object.keys(locales[locale] ?? {}).sort()).toEqual(Object.keys(en).sort());
	});

	it.each(translatedLocales)('%s keeps every {placeholder} from en.json', (locale) => {
		for (const [key, value] of Object.entries(en)) {
			expect(placeholders(locales[locale]?.[key] ?? ''), key).toEqual(placeholders(value));
		}
	});
});

it('message keys are unique across catalogs', () => {
	const seen = new Map<string, string>();
	for (const [area, locales] of byArea) {
		for (const key of Object.keys(locales.en ?? {})) {
			expect(seen.get(key), `${key} is defined in both "${seen.get(key)}" and "${area}"`).toBe(
				undefined
			);
			seen.set(key, area);
		}
	}
});
