export interface APIKey {
	id: number;
	userId: number;
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

export type APIKeyCapabilityKey =
	| 'canAccessAPI'
	| 'canAccessLLMProxy'
	| 'canAccessSkills'
	| 'canAccessDeviceScans';
export type APIKeyCreatableCapabilityKey = Exclude<APIKeyCapabilityKey, 'canAccessAPI'>;

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
	},
	{
		key: 'canAccessDeviceScans',
		label: 'Device scan access',
		shortLabel: 'Scans',
		description: 'Grants this key access to submit and read device scans.'
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
