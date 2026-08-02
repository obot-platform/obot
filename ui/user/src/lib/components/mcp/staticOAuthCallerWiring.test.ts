import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const callers = [
	{
		name: 'catalog entries view',
		url: new URL('../../../routes/mcp-catalog/EntriesView.svelte', import.meta.url),
		deletesCredentials: true
	},
	{
		name: 'server actions',
		url: new URL('./McpServerActions.svelte', import.meta.url),
		deletesCredentials: false
	},
	{
		name: 'catalog entry form',
		url: new URL('../admin/McpServerEntryForm.svelte', import.meta.url),
		deletesCredentials: true
	}
];

for (const caller of callers) {
	test(`${caller.name} wires the shared static OAuth test and save flow`, async () => {
		const source = await readFile(caller.url, 'utf8');
		const expectedOperations = [
			'StaticOAuthConfigureModal',
			'staticOAuthSaveWasCommitted',
			'onStartTest=',
			'onGetTest=',
			'onSave=',
			'UserService.startWorkspaceMCPCatalogEntryOAuthCredentialTest',
			'AdminService.startMCPCatalogEntryOAuthCredentialTest',
			'UserService.getWorkspaceMCPCatalogEntryOAuthCredentialTest',
			'AdminService.getMCPCatalogEntryOAuthCredentialTest',
			'UserService.setWorkspaceMCPCatalogEntryOAuthCredentials',
			'AdminService.setMCPCatalogEntryOAuthCredentials',
			'UserService.replaceWorkspaceMCPCatalogEntryOAuthCredentials',
			'AdminService.replaceMCPCatalogEntryOAuthCredentials'
		];

		for (const operation of expectedOperations) {
			assert.ok(source.includes(operation), `${caller.name} is missing ${operation}`);
		}
		assert.ok(
			!source.includes("= { configured: false, callbackURL: '' }"),
			`${caller.name} must not fabricate an unconfigured status when the status request fails`
		);

		if (caller.deletesCredentials) {
			for (const operation of [
				'onDelete=',
				'UserService.deleteWorkspaceMCPCatalogEntryOAuthCredentials',
				'AdminService.deleteMCPCatalogEntryOAuthCredentials'
			]) {
				assert.ok(source.includes(operation), `${caller.name} is missing ${operation}`);
			}
			assert.match(
				source,
				/onDelete=\{(?:handleDeleteOAuth|async \(expectedGeneration\))/,
				`${caller.name} must clear the exact OAuth application generation it loaded`
			);
			assert.ok(
				source.includes('expectedGeneration'),
				`${caller.name} must forward the reviewed OAuth application generation`
			);
		}
	});
}

test('shared static OAuth modal keeps submitted values immutable and explains entry-wide clear', async () => {
	const modal = await readFile(
		new URL('./StaticOAuthConfigureModal.svelte', import.meta.url),
		'utf8'
	);

	assert.match(modal, /id="clientID"[\s\S]*?disabled=\{loading\}/);
	assert.match(modal, /name="clientSecret"[\s\S]*?disabled=\{loading\}/);
	assert.match(modal, /all deployments remain[\s\S]*?all Users must reconnect/i);
	assert.match(modal, /onDelete\?: \(expectedGeneration: string\)/);
	assert.match(modal, /const expectedGeneration = oauthStatus\?\.generation/);
	assert.match(modal, /Reload the OAuth application status before clearing credentials/);
});

test('static OAuth Clear services send the reviewed generation in a DELETE body', async () => {
	for (const [name, url] of [
		['admin', new URL('../../services/admin/operations.ts', import.meta.url)],
		['workspace', new URL('../../services/user/operations.ts', import.meta.url)]
	] as const) {
		const source = await readFile(url, 'utf8');
		assert.match(source, /doWithBody\(\s*'DELETE',[\s\S]*?\{ expectedGeneration \}/);
		assert.match(
			source,
			/expectedGeneration: string/,
			`${name} Clear service must require a generation`
		);
	}
});
