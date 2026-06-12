export interface APIKey {
	id: number;
	userId: number;
	name: string;
	description?: string;
	canAccessAPI: boolean;
	canAccessLLMProxy: boolean;
	canAccessSkills: boolean;
	createdAt: string;
	lastUsedAt?: string;
	expiresAt?: string;
	mcpServerIds?: string[];
}

export type APIKeyCapabilityKey = 'canAccessAPI' | 'canAccessLLMProxy' | 'canAccessSkills';

export const API_KEY_CAPABILITIES = [
	{
		key: 'canAccessAPI',
		label: 'API access',
		shortLabel: 'API',
		description: 'Grants this key access to the Obot API using your user role permissions.'
	},
	{
		key: 'canAccessLLMProxy',
		label: 'LLM proxy access',
		shortLabel: 'LLM',
		description: 'Grants this key access to LLM proxy endpoints.'
	},
	{
		key: 'canAccessSkills',
		label: 'Skill access',
		shortLabel: 'Skills',
		description: 'Grants this key read-only access for skill discovery and downloads.'
	}
] as const satisfies ReadonlyArray<{
	key: APIKeyCapabilityKey;
	label: string;
	shortLabel: string;
	description: string;
}>;

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
}

export interface APIKeyCreateResponse extends APIKey {
	key: string; // Only shown once on creation
}
