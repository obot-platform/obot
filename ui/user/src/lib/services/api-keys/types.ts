import { m } from '$lib/i18n';
import { agentKeyMask } from '$lib/services/agent-identities/helpers';

export interface APIKey {
	id: number;
	userId: number;
	// Set instead of userId for an agent's key, which is managed on the agent.
	agentId?: string;
	name: string;
	description?: string;
	canAccessAPI: boolean;
	canAccessLLMProxy: boolean;
	canAccessSkills: boolean;
	canAccessDeviceScans: boolean;
	createdAt: string;
	lastUsedAt?: string;
	expiresAt?: string;
	mcpServerIds?: string[];
}

// apiKeyMask is how a key is shown: its secret is never available after creation. A user's key is
// ok1-<userId>-<keyId>, and an agent's is oak1-<agentId>-<keyId>.
export function apiKeyMask(key: Pick<APIKey, 'id' | 'userId' | 'agentId'>): string {
	return key.agentId
		? agentKeyMask({ agentId: key.agentId, id: key.id })
		: `ok1-${key.userId}-${key.id}-*****`;
}

export type APIKeyCapabilityKey =
	| 'canAccessAPI'
	| 'canAccessLLMProxy'
	| 'canAccessSkills'
	| 'canAccessDeviceScans';
export type APIKeyCreatableCapabilityKey = Exclude<APIKeyCapabilityKey, 'canAccessAPI'>;

export const API_KEY_CAPABILITIES = [
	{
		key: 'canAccessAPI',
		label: m.identity_access_api_keys_capability_api_label(),
		shortLabel: 'API',
		description: m.identity_access_api_keys_capability_api_description()
	},
	{
		key: 'canAccessLLMProxy',
		label: m.identity_access_api_keys_capability_llm_label(),
		shortLabel: 'LLM',
		description: m.identity_access_api_keys_capability_llm_description()
	},
	{
		key: 'canAccessSkills',
		label: m.identity_access_api_keys_capability_skills_label(),
		shortLabel: m.identity_access_api_keys_capability_skills_short(),
		description: m.identity_access_api_keys_capability_skills_description()
	},
	{
		key: 'canAccessDeviceScans',
		label: m.identity_access_api_keys_capability_scans_label(),
		shortLabel: m.identity_access_api_keys_capability_scans_short(),
		description: m.identity_access_api_keys_capability_scans_description()
	}
] as const satisfies ReadonlyArray<{
	key: APIKeyCapabilityKey;
	label: string;
	shortLabel: string;
	description: string;
}>;

export const API_KEY_CREATABLE_CAPABILITIES = API_KEY_CAPABILITIES.filter(
	(
		capability
	): capability is Extract<
		(typeof API_KEY_CAPABILITIES)[number],
		{ key: APIKeyCreatableCapabilityKey }
	> => capability.key !== 'canAccessAPI'
);

export function getAPIKeyCapabilityLabels(apiKey: Pick<APIKey, APIKeyCapabilityKey>): string[] {
	return API_KEY_CAPABILITIES.filter((capability) => apiKey[capability.key]).map(
		(capability) => capability.shortLabel
	);
}

export interface APIKeyCreateRequest {
	name: string;
	description?: string;
	expiresAt?: string;
	mcpServerIds: string[];
	canAccessAPI?: boolean;
	canAccessLLMProxy?: boolean;
	canAccessSkills?: boolean;
	canAccessDeviceScans?: boolean;
}

export interface APIKeyCreateResponse extends APIKey {
	key: string; // Only shown once on creation
}
