import type {
	AgentAPIKey,
	AgentDelegationApprovalRequest,
	AgentDelegationSetup,
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

// What a user set up through an agent: one vMCP that needs setup, with a value to enter, a set value,
// an account to connect and a component that needs nothing, and one vMCP that is ready.
export function createDelegationSetup(): AgentDelegationSetup {
	return {
		vmcps: [
			{
				vmcpID: 'vmcp1dev',
				displayName: 'Developer tools',
				ready: false,
				components: [
					{
						componentID: 'c-github',
						name: 'GitHub',
						covered: false,
						ready: false,
						reasons: [{ componentID: 'c-github', code: 'configuration_required', field: 'TOKEN' }],
						configuration: [
							{
								key: 'TOKEN',
								name: 'Token',
								description: 'A personal access token',
								sensitive: false,
								required: true,
								usage: 'env',
								set: false
							},
							{
								key: 'ORG',
								name: 'Organization',
								description: '',
								sensitive: false,
								required: true,
								usage: 'header',
								set: true
							}
						],
						oauth: 'notRequired'
					},
					{
						componentID: 'c-slack',
						name: 'Slack',
						covered: false,
						ready: false,
						reasons: [{ componentID: 'c-slack', code: 'oauth_required' }],
						configuration: [],
						oauth: 'needsAuthorization'
					},
					{
						componentID: 'c-clock',
						name: 'Clock',
						covered: true,
						coveredBy: 'nothing',
						ready: true,
						configuration: [],
						oauth: 'notRequired'
					}
				]
			},
			{
				vmcpID: 'vmcp1docs',
				displayName: 'Docs',
				ready: true,
				components: [
					{
						componentID: 'c-notion',
						name: 'Notion',
						covered: false,
						ready: true,
						configuration: [],
						oauth: 'connected'
					}
				]
			}
		]
	};
}

export function createDelegationRequest(
	overrides: Partial<AgentDelegationApprovalRequest> = {}
): AgentDelegationApprovalRequest {
	return {
		id: 'adr1req',
		agent: { id: '7', displayName: 'Build Bot', description: 'Runs builds', status: 'active' },
		expiresAt: '2026-01-01T00:10:00.000Z',
		access: {
			vmcps: [
				{
					vmcpID: 'vmcp1dev',
					vmcpDisplayName: 'Developer tools',
					profiles: [{ id: 'vp1read', name: 'Read only' }]
				}
			],
			modelAccessPolicies: [{ id: 'map1team', displayName: 'Team models' }],
			skillAccessRules: []
		},
		...overrides
	};
}
