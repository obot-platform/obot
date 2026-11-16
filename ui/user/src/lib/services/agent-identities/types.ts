// Types of the agent identities API (apiclient/types/agentidentity.go). IDs are decimal strings.

export type AgentIdentityStatus = 'active' | 'disabled' | 'deleted';

export interface AgentIdentityManifest {
	displayName: string;
	description?: string;
}

export interface AgentIdentity extends AgentIdentityManifest {
	id: string;
	created: string;
	deleted?: string;
	creatorUserID?: string;
	status: AgentIdentityStatus;
	disabledAt?: string;
	disabledReason?: string;
	activeAPIKeyCount: number;
}

export interface AgentAPIKey {
	id: number;
	agentId: string;
	name: string;
	description?: string;
	canAccessSkills: boolean;
	canAccessLLMProxy: boolean;
	// Always present. "*" means every vMCP the agent is granted.
	mcpServerIds: string[];
	createdAt: string;
	lastUsedAt?: string;
	expiresAt?: string;
	revokedAt?: string;
}

// An agent key never grants the API or device scans, so the request has no field for either.
// A key needs at least one scope; the server answers 400 otherwise.
export interface AgentAPIKeyCreateRequest {
	name: string;
	description?: string;
	expiresAt?: string;
	canAccessSkills?: boolean;
	canAccessLLMProxy?: boolean;
	mcpServerIds?: string[];
}

export interface AgentAPIKeyCreateResponse extends AgentAPIKey {
	key: string; // Only shown once on creation
}

export type AgentSubjectMatch = 'explicit' | 'kindWildcard' | 'wildcard';

export interface AgentIdentityVMCPProfileAccess {
	vmcpID: string;
	vmcpDisplayName?: string;
	profileID?: string;
	profileName: string;
	match: AgentSubjectMatch;
}

export interface AgentIdentityPolicyAccess {
	id: string;
	displayName?: string;
	match: AgentSubjectMatch;
}

export type AgentProblemCode = 'no_model_access_policy' | 'no_skill_access_rule';

export interface AgentIdentityAccess {
	agentID: string;
	vmcpProfiles: AgentIdentityVMCPProfileAccess[];
	modelAccessPolicies: AgentIdentityPolicyAccess[];
	skillAccessRules: AgentIdentityPolicyAccess[];
	problems: { code: AgentProblemCode }[];
}

export type AgentReadinessCode =
	| 'configuration_required'
	| 'oauth_required'
	| 'oauth_invalid'
	| 'selection_ambiguous';

export interface AgentReadinessReason {
	componentID?: string;
	code: AgentReadinessCode;
	// The configuration key that is missing, never its value.
	field?: string;
	profileID?: string;
	urlProfileID?: string;
}

export interface AgentVMCPReadiness {
	vmcpID: string;
	vmcpDisplayName?: string;
	ready: boolean;
	disabled: boolean;
	explicitProfileID?: string;
	wildcardProfileID?: string;
	reasons: AgentReadinessReason[];
	pendingSync?: boolean;
}

// An entry of the admin change audit (apiclient/types/adminauditevent.go).
export interface AdminAuditEvent {
	id: number;
	createdAt: string;
	actor: { actorType: string; id?: string; displayName?: string };
	action: string;
	resourceKind: string;
	resourceID: string;
	parentID?: string;
	details?: Record<string, unknown>;
}

export interface AdminAuditEventList {
	items: AdminAuditEvent[] | null;
	total: number;
	limit: number;
	offset: number;
}

// PrincipalRef is the typed owner or caller of a row: a user or an agent.
export interface PrincipalRef {
	kind: 'user' | 'agent';
	id: string;
}
