// @ts-expect-error Node runs this co-located TypeScript test directly and requires the extension.
import * as staticOAuthCredentialTestState from './staticOAuthCredentialTestState.ts';
import assert from 'node:assert/strict';
import test from 'node:test';

const {
	beginStaticOAuthCredentialTest,
	canSaveStaticOAuthCredentials,
	failStaticOAuthCredentialTest,
	invalidateStaticOAuthCredentialTest,
	safeStaticOAuthAuthorizationURL,
	scheduleStaticOAuthCredentialTestExpiry,
	staticOAuthSaveWasCommitted,
	succeedStaticOAuthCredentialTest
} = staticOAuthCredentialTestState;

test('authorization navigation accepts only HTTP URLs with a hostname', () => {
	assert.equal(
		safeStaticOAuthAuthorizationURL('https://provider.example/authorize?client_id=client'),
		'https://provider.example/authorize?client_id=client'
	);
	assert.equal(
		safeStaticOAuthAuthorizationURL('http://127.0.0.1:8080/authorize'),
		'http://127.0.0.1:8080/authorize'
	);
	assert.equal(safeStaticOAuthAuthorizationURL('javascript:alert(document.domain)'), undefined);
	assert.equal(safeStaticOAuthAuthorizationURL('not a URL'), undefined);
});

test('save requires a successful proof for the exact tested credentials', () => {
	const pending = beginStaticOAuthCredentialTest(' client-id ', ' client-secret ');

	assert.equal(canSaveStaticOAuthCredentials(pending, 'client-id', 'client-secret'), false);

	const expiresAt = '2026-08-02T02:00:00.000Z';
	const succeeded = succeedStaticOAuthCredentialTest(pending, 'proof', expiresAt);
	assert.equal(
		canSaveStaticOAuthCredentials(
			succeeded,
			' client-id ',
			' client-secret ',
			Date.parse(expiresAt) - 1
		),
		true
	);
	assert.equal(
		canSaveStaticOAuthCredentials(
			succeeded,
			'changed-id',
			'client-secret',
			Date.parse(expiresAt) - 1
		),
		false
	);
	assert.equal(
		canSaveStaticOAuthCredentials(
			succeeded,
			'client-id',
			'changed-secret',
			Date.parse(expiresAt) - 1
		),
		false
	);
	assert.equal(
		canSaveStaticOAuthCredentials(
			succeeded,
			' client-id ',
			' client-secret ',
			Date.parse(expiresAt)
		),
		false
	);
});

test('a successful test preserves the proof as an exact opaque value', () => {
	const succeeded = succeedStaticOAuthCredentialTest(
		beginStaticOAuthCredentialTest('client-id', 'client-secret'),
		' proof ',
		'2026-08-02T02:00:00.000Z'
	);

	assert.equal(succeeded.status, 'succeeded');
	if (succeeded.status === 'succeeded') {
		assert.equal(succeeded.proof, ' proof ');
	}
});

test('editing either value invalidates the proof even if the original value is restored', () => {
	const succeeded = succeedStaticOAuthCredentialTest(
		beginStaticOAuthCredentialTest('client-id', 'client-secret'),
		'proof',
		'2026-08-02T02:00:00.000Z'
	);
	const invalidated = invalidateStaticOAuthCredentialTest(succeeded);

	assert.equal(canSaveStaticOAuthCredentials(invalidated, 'client-id', 'client-secret'), false);
});

test('a failed test and a new test cannot reuse an earlier proof', () => {
	const succeeded = succeedStaticOAuthCredentialTest(
		beginStaticOAuthCredentialTest('client-id', 'client-secret'),
		'old-proof',
		'2026-08-02T02:00:00.000Z'
	);
	const failed = failStaticOAuthCredentialTest(succeeded, 'token_exchange_failed');
	const restarted = beginStaticOAuthCredentialTest('client-id', 'client-secret');

	assert.equal(canSaveStaticOAuthCredentials(failed, 'client-id', 'client-secret'), false);
	assert.equal(canSaveStaticOAuthCredentials(restarted, 'client-id', 'client-secret'), false);
});

test('only the exact proof receipt confirms an ambiguous save', async () => {
	const receipt = 'c1cda26362828b69266512052b97cb3729e3b052e4ade47c0a1e3383defe73c7';
	assert.equal(
		await staticOAuthSaveWasCommitted(
			{ configured: true, clientID: 'client-id', generation: receipt },
			'client-id',
			'proof'
		),
		true
	);
	assert.equal(
		await staticOAuthSaveWasCommitted(
			{ configured: true, clientID: 'other-client', generation: receipt },
			'client-id',
			'proof'
		),
		false
	);
	assert.equal(
		await staticOAuthSaveWasCommitted(
			{ configured: true, clientID: 'client-id', generation: 'other-generation' },
			'client-id',
			'proof'
		),
		false
	);
	assert.equal(
		await staticOAuthSaveWasCommitted(
			{ configured: false, clientID: 'client-id', generation: receipt },
			'client-id',
			'proof'
		),
		false
	);
});

test('server expiry invalidates the proof at the authoritative time', (t) => {
	const now = Date.parse('2026-08-02T01:00:00.000Z');
	t.mock.timers.enable({ apis: ['setTimeout'], now });
	let expired = false;
	scheduleStaticOAuthCredentialTestExpiry(
		new Date(now + 1_000).toISOString(),
		() => {
			expired = true;
		},
		now
	);

	t.mock.timers.tick(999);
	assert.equal(expired, false);
	t.mock.timers.tick(1);
	assert.equal(expired, true);
});

test('expiry cleanup prevents an unmounted modal from being updated', (t) => {
	const now = Date.parse('2026-08-02T01:00:00.000Z');
	t.mock.timers.enable({ apis: ['setTimeout'], now });
	let expiryCallbacks = 0;
	const cleanup = scheduleStaticOAuthCredentialTestExpiry(
		new Date(now + 1_000).toISOString(),
		() => {
			expiryCallbacks += 1;
		},
		now
	);

	cleanup();
	t.mock.timers.tick(1_000);
	assert.equal(expiryCallbacks, 0);
});
