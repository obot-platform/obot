import catalog from '$lib/data/credential-rules.json';
import type { MCPCatalogEntryFieldManifest } from '$lib/services';

export const credentialCatalog = catalog;
export const credentialActions = ['block', 'redact', 'allow'] as const;
export type CredentialAction = (typeof credentialActions)[number];
export const credentialDefaultKey = 'CREDENTIAL_DEFAULT_ACTION';
export const credentialListKeys = {
	block: 'CREDENTIAL_BLOCK_RULES',
	redact: 'CREDENTIAL_REDACT_RULES',
	allow: 'CREDENTIAL_ALLOW_RULES'
};
export const credentialKeys = [credentialDefaultKey, ...Object.values(credentialListKeys)];
export const actionLabels = { block: 'Block', redact: 'Redact', allow: 'Allow' };
export const actionHelp = {
	block: 'Reject the entire message when a credential matches.',
	redact: 'Replace detected credentials before forwarding the message.',
	allow: 'Detected credentials pass through unchanged unless an override applies.'
};
export type CredentialPolicy = {
	defaultAction: string;
	overrides: { ruleId: string; action: CredentialAction }[];
};
export function readCredentialPolicy(config: MCPCatalogEntryFieldManifest[]): CredentialPolicy {
	return {
		defaultAction: config.find((field) => field.key === credentialDefaultKey)?.value || 'block',
		overrides: credentialActions.flatMap((action) =>
			(config.find((field) => field.key === credentialListKeys[action])?.value || '')
				.split(',')
				.flatMap((id) => {
					const ruleId = id.trim();
					return ruleId ? [{ ruleId, action }] : [];
				})
		)
	};
}
export function validateCredentialPolicy(policy: CredentialPolicy): string | undefined {
	if (!credentialActions.includes(policy.defaultAction as CredentialAction))
		return 'Choose a valid default action: Block, Redact, or Allow.';
	const seen = new Set<string>();
	for (const override of policy.overrides) {
		if (!catalog.rules.some((rule) => rule.id === override.ruleId))
			return `Unsupported credential rule ${override.ruleId}. Remove this override or use a compatible filter version.`;
		if (seen.has(override.ruleId))
			return `Duplicate or conflicting assignment for ${override.ruleId}. Remove the duplicate override.`;
		if (!credentialActions.includes(override.action))
			return 'Choose a valid override action: Block, Redact, or Allow.';
		seen.add(override.ruleId);
	}
}
export function credentialEnvironment(policy: CredentialPolicy): Record<string, string> {
	const error = validateCredentialPolicy(policy);
	if (error) throw new Error(error);
	const ruleIds: Partial<Record<CredentialAction, string[]>> = {};
	for (const override of policy.overrides) {
		(ruleIds[override.action] ??= []).push(override.ruleId);
	}
	return {
		[credentialDefaultKey]: policy.defaultAction,
		...Object.fromEntries(
			credentialActions.map((action) => [
				credentialListKeys[action],
				(ruleIds[action] ?? []).sort().join(',')
			])
		)
	};
}
export function credentialMutationRequired(policy: CredentialPolicy): boolean {
	return (
		policy.defaultAction === 'redact' ||
		policy.overrides.some((override) => override.action === 'redact')
	);
}
export function writeCredentialPolicy(
	config: MCPCatalogEntryFieldManifest[],
	policy: CredentialPolicy
) {
	const values = credentialEnvironment(policy);
	return [
		...config.filter((field) => !credentialKeys.includes(field.key)),
		...credentialKeys.map((key) => ({
			description: '',
			required: false,
			sensitive: false,
			...config.find((field) => field.key === key),
			key,
			name: key,
			value: values[key]
		}))
	];
}
