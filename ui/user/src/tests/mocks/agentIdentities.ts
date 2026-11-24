import type {
	AgentAPIKey,
	AgentIdentity,
	AgentIdentityAccess,
	AgentVMCPReadiness
} from '$lib/services/agent-identities/types';

export function createAgent(overrides: Partial<AgentIdentity> = {}): AgentIdentity {
	return {
		id: '7',
		created: '2026-01-01T00:00:00.000Z',
		displayName: 'Build Bot',
		description: 'Runs builds',
		creatorUserID: '1',
		status: 'active',
		activeAPIKeyCount: 1,
		...overrides
	};
}

export function createAgentKey(overrides: Partial<AgentAPIKey> = {}): AgentAPIKey {
	return {
		id: 11,
		agentId: '7',
		name: 'ci key',
		canAccessSkills: false,
		canAccessLLMProxy: true,
		mcpServerIds: ['*'],
		createdAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

export const emptyAgentAccess: AgentIdentityAccess = {
	agentID: '7',
	vmcpProfiles: [],
	modelAccessPolicies: [],
	skillAccessRules: [],
	problems: []
};

export const readyVMCP: AgentVMCPReadiness = {
	vmcpID: 'vmcp1mail',
	vmcpDisplayName: 'Mail vMCP',
	ready: false,
	disabled: false,
	reasons: [{ code: 'oauth_required', componentID: 'gmail' }],
	preconfiguredComponents: []
};
