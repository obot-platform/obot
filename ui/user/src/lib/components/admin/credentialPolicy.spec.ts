import {
	credentialEnvironment,
	credentialMutationRequired,
	readCredentialPolicy,
	validateCredentialPolicy,
	writeCredentialPolicy,
	type CredentialPolicy
} from './credentialPolicy';
import { describe, expect, it } from 'vitest';

describe('credential policy', () => {
	it('defaults to block and explicitly clears all exception lists', () => {
		expect(credentialEnvironment(readCredentialPolicy([]))).toEqual({
			CREDENTIAL_DEFAULT_ACTION: 'block',
			CREDENTIAL_ALLOW_RULES: '',
			CREDENTIAL_BLOCK_RULES: '',
			CREDENTIAL_REDACT_RULES: ''
		});
	});
	it('round-trips explicit assignments and sorts exact IDs', () => {
		const policy: CredentialPolicy = {
			defaultAction: 'allow',
			overrides: [
				{ ruleId: 'np.slack.2', action: 'allow' },
				{ ruleId: 'np.github.1', action: 'allow' }
			]
		};
		const env = credentialEnvironment(policy);
		expect(env.CREDENTIAL_ALLOW_RULES).toBe('np.github.1,np.slack.2');
		const fields = writeCredentialPolicy([], policy);
		expect(readCredentialPolicy(fields).overrides).toHaveLength(2);
		const changed = { ...readCredentialPolicy(fields), defaultAction: 'block' };
		expect(credentialEnvironment(changed).CREDENTIAL_ALLOW_RULES).toBe(env.CREDENTIAL_ALLOW_RULES);
		expect(credentialEnvironment({ ...changed, overrides: [] }).CREDENTIAL_ALLOW_RULES).toBe('');
	});
	it.each([
		{ defaultAction: 'none', overrides: [] },
		{ defaultAction: 'block', overrides: [{ ruleId: 'unknown', action: 'allow' }] },
		{
			defaultAction: 'block',
			overrides: [
				{ ruleId: 'np.github.1', action: 'allow' },
				{ ruleId: 'np.github.1', action: 'redact' }
			]
		},
		{ defaultAction: 'block', overrides: [{ ruleId: 'np.github.1', action: 'none' }] }
	])('rejects invalid saved policies: %j', (policy) => {
		expect(validateCredentialPolicy(policy as CredentialPolicy)).toBeTruthy();
		expect(() => credentialEnvironment(policy as CredentialPolicy)).toThrow();
	});
	it('requires mutation for either redaction path', () => {
		expect(credentialMutationRequired({ defaultAction: 'block', overrides: [] })).toBe(false);
		expect(credentialMutationRequired({ defaultAction: 'redact', overrides: [] })).toBe(true);
		expect(
			credentialMutationRequired({
				defaultAction: 'allow',
				overrides: [{ ruleId: 'np.github.1', action: 'redact' }]
			})
		).toBe(true);
	});
});
