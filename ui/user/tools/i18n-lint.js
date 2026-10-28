import { readFileSync } from 'node:fs';
import { dirname, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

/** Check the catalogs configured in Paraglide without relying on generated output. */
export function lintCatalogs(root) {
	const settings = JSON.parse(readFileSync(resolve(root, 'project.inlang/settings.json'), 'utf8'));
	const patterns = settings['plugin.inlang.messageFormat'].pathPattern;
	const errors = [];
	const catalogs = new Map(settings.locales.map((locale) => [locale, new Map()]));

	for (const locale of settings.locales) {
		const catalog = catalogs.get(locale);
		for (const pattern of Array.isArray(patterns) ? patterns : [patterns]) {
			const path = resolve(root, pattern.replaceAll('{locale}', locale));
			const label = relative(root, path);
			let messages;
			try {
				messages = JSON.parse(readFileSync(path, 'utf8'));
			} catch (error) {
				errors.push(`${label}: ${error.message}`);
				continue;
			}
			if (!messages || typeof messages !== 'object' || Array.isArray(messages)) {
				errors.push(`${label}: expected a message object`);
				continue;
			}
			for (const [key, value] of Object.entries(messages)) {
				if (key === '$schema') continue;
				if (catalog.has(key)) {
					errors.push(`${label}: duplicate message "${key}" (also in ${catalog.get(key)})`);
				}
				// Empty prefixes/suffixes are intentional in languages with different word order.
				if (typeof value !== 'string') {
					errors.push(`${label}: message "${key}" must be a string`);
				}
				catalog.set(key, label);
			}
		}
	}

	const keys = new Set([...catalogs.values()].flatMap((catalog) => [...catalog.keys()]));
	for (const key of [...keys].sort()) {
		for (const [locale, catalog] of catalogs) {
			if (!catalog.has(key)) errors.push(`message "${key}" is missing in ${locale}`);
		}
	}
	return errors;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
	try {
		const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
		const errors = lintCatalogs(root);
		if (errors.length) {
			console.error(errors.join('\n'));
			process.exitCode = 1;
		}
	} catch (error) {
		console.error(error.message);
		process.exitCode = 1;
	}
}
