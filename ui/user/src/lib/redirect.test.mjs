import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

const outDir = join(tmpdir(), 'obot-user-redirect-test');

async function loadModule() {
	rmSync(outDir, { recursive: true, force: true });
	mkdirSync(outDir, { recursive: true });
	execFileSync(
		'./node_modules/.bin/tsc',
		[
			'src/lib/redirect.ts',
			'--ignoreConfig',
			'--target',
			'ES2022',
			'--module',
			'NodeNext',
			'--moduleResolution',
			'NodeNext',
			'--outDir',
			outDir,
			'--skipLibCheck'
		],
		{ stdio: 'inherit' }
	);
	return import(`${outDir}/redirect.js`);
}

test('safeRedirectPath accepts app-relative paths only', async () => {
	const { safeRedirectPath } = await loadModule();

	assert.equal(safeRedirectPath('/mcp-servers'), '/mcp-servers');
	assert.equal(safeRedirectPath('/obot/mcp-servers'), '/obot/mcp-servers');
	assert.equal(safeRedirectPath('/mcp-servers', '/obot'), '/obot/mcp-servers');
	assert.equal(safeRedirectPath('/obot/mcp-servers', '/obot'), '/obot/mcp-servers');
	assert.equal(safeRedirectPath('https://evil.example'), null);
	assert.equal(safeRedirectPath('//evil.example'), null);
	assert.equal(safeRedirectPath('/\\evil.example'), null);
	assert.equal(safeRedirectPath('admin/dashboard'), null);
});
