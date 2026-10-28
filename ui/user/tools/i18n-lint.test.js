import { lintCatalogs } from './i18n-lint.js';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';

function fixture(t, catalogs) {
	const root = mkdtempSync(join(tmpdir(), 'obot-i18n-'));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	mkdirSync(join(root, 'project.inlang'));
	mkdirSync(join(root, 'messages/area'), { recursive: true });
	writeFileSync(
		join(root, 'project.inlang/settings.json'),
		JSON.stringify({
			baseLocale: 'en',
			locales: ['en', 'ja'],
			'plugin.inlang.messageFormat': {
				pathPattern: ['./messages/{locale}.json', './messages/area/{locale}.json']
			}
		})
	);
	for (const [path, messages] of Object.entries(catalogs)) {
		writeFileSync(join(root, 'messages', path), JSON.stringify(messages));
	}
	return root;
}

const complete = {
	'en.json': { $schema: 'schema', greeting: 'Hello' },
	'ja.json': { greeting: 'こんにちは' },
	'area/en.json': { prefix: 'before' },
	'area/ja.json': { prefix: '' }
};

test('accepts complete catalogs, including intentionally empty translations', (t) => {
	assert.deepEqual(lintCatalogs(fixture(t, complete)), []);
});

test('rejects missing translations and keys without an English source', (t) => {
	const root = fixture(t, { ...complete, 'ja.json': { extra: '余分' } });
	assert.deepEqual(lintCatalogs(root), [
		'message "extra" is missing in en',
		'message "greeting" is missing in ja'
	]);
});

test('rejects duplicate keys across areas and non-string translations', (t) => {
	const root = fixture(t, { ...complete, 'area/ja.json': { prefix: null, greeting: '重複' } });
	const errors = lintCatalogs(root);
	assert.ok(errors.some((error) => error.includes('message "prefix" must be a string')));
	assert.ok(errors.some((error) => error.includes('duplicate message "greeting"')));
});

test('rejects missing and malformed catalog files', (t) => {
	const root = fixture(t, complete);
	rmSync(join(root, 'messages/ja.json'));
	writeFileSync(join(root, 'messages/area/ja.json'), '{');
	const errors = lintCatalogs(root);
	assert.ok(errors.some((error) => error.startsWith('messages/ja.json:')));
	assert.ok(errors.some((error) => error.startsWith('messages/area/ja.json:')));
});
