// Types of the agent identities API (apiclient/types/agentidentity.go). IDs are decimal strings.
import type { AccessControlRuleSubject } from '$lib/services/user/types';

export type AgentIdentityStatus = 'active' | 'disabled' | 'deleted';

export interface AgentIdentityManifest {
	displayName: string;
	description?: string;
	// An update replaces the whole list, so one that leaves it out removes every entry. Left out when there is none.
	profiles?: AgentProfile[];
	// The hosts, each with an optional port, that the agent may send users back to after they approve it. An update
	// replaces the whole list. Left out when there is none.
	allowedReturnHosts?: string[];
}

// Users an agent may act for (apiclient/types/agentprofile.go). For a user one matches, the agent has its own grants.
export interface AgentProfile {
	// Assigned by the server; omit it for a new entry.
	id?: string;
	name: string;
	subjects: AccessControlRuleSubject[];
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

// A vMCP profile that names an agent.
export interface AgentIdentityVMCPProfileAccess {
	vmcpID: string;
	vmcpDisplayName?: string;
	profileID?: string;
	profileName: string;
}

// A model access policy or skill access rule that names an agent.
export interface AgentIdentityPolicyAccess {
	id: string;
	displayName?: string;
}

export type AgentProblemCode = 'no_model_access_policy' | 'no_skill_access_rule';

export interface AgentIdentityAccess {
	agentID: string;
	vmcpProfiles: AgentIdentityVMCPProfileAccess[];
	modelAccessPolicies: AgentIdentityPolicyAccess[];
	skillAccessRules: AgentIdentityPolicyAccess[];
	problems: { code: AgentProblemCode }[];
}

export type AgentReadinessCode = 'configuration_required' | 'oauth_required' | 'oauth_invalid';

// Every reason is fixed in the component's settings in the vMCP editor: a configuration value fixed on
// the vMCP (or on the component's catalog entry), or the component's preconfigured OAuth.
export interface AgentReadinessReason {
	componentID?: string;
	code: AgentReadinessCode;
	// The configuration key that is missing, never its value.
	field?: string;
}

export interface AgentVMCPReadiness {
	vmcpID: string;
	vmcpDisplayName?: string;
	ready: boolean;
	disabled: boolean;
	reasons: AgentReadinessReason[];
	// The components the agent is granted whose preconfigured OAuth on the vMCP works, by ID.
	preconfiguredComponents: string[];
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

// A user's approval for an agent to act on their behalf (apiclient/types/agentdelegation.go). Administrators list
// the delegations of an agent, never what the user set up through it.
export interface AgentDelegation {
	id: string;
	agent: { id: string; displayName: string; description?: string; status: AgentIdentityStatus };
	user: { id: string; email?: string; displayName?: string; status?: string };
	firstApprovedAt: string;
	lastApprovedAt: string;
	// When the agent last got a token from the delegation.
	lastUsedAt?: string;
}

// An agent's request to act on behalf of the signed-in user, as the user who can approve it sees it
// (GET /api/agent-delegation-requests/{id}): the agent, and what it will be able to do for them.
export interface AgentDelegationApprovalRequest {
	id: string;
	agent: AgentDelegation['agent'];
	expiresAt: string;
	// The user's existing delegation of the agent, if they approved it before. Approving again keeps it.
	delegationID?: string;
	access: AgentDelegationAccess;
}

// What an agent may do for a user one of its profiles matches: the agent's own grants, which are the
// same for every such user.
export interface AgentDelegationAccess {
	vmcps: { vmcpID: string; vmcpDisplayName?: string; profiles: { id: string; name: string }[] }[];
	modelAccessPolicies: AgentIdentityPolicyAccess[];
	skillAccessRules: AgentIdentityPolicyAccess[];
}

// Where to send the user's browser after they approve or cancel a request. Only the server builds it.
export interface AgentDelegationRedirect {
	redirectURL: string;
}

// The setup API is the same while a user approves a request and afterwards, for one of their
// delegations; only the prefix of its routes differs.
export type AgentDelegationSetupScope =
	| { kind: 'request'; requestId: string }
	| { kind: 'delegation'; delegationId: string };

export type AgentDelegationSetupOAuthState =
	| 'notRequired'
	| 'preconfigured'
	| 'connected'
	| 'needsAuthorization';

// What a user has set up through an agent, and still needs to, for each vMCP the agent may use for
// them. It never carries a value or a token.
export interface AgentDelegationSetup {
	vmcps: AgentDelegationSetupVMCP[];
}

export interface AgentDelegationSetupVMCP {
	vmcpID: string;
	displayName?: string;
	ready: boolean;
	components: AgentDelegationSetupComponent[];
}

export interface AgentDelegationSetupComponent {
	componentID: string;
	name?: string;
	// The component needs nothing from the user: the vMCP fixes it, the vMCP's preconfigured OAuth
	// covers it, or it needs nothing at all.
	covered: boolean;
	coveredBy?: 'fixed' | 'preconfiguredOAuth' | 'nothing';
	ready: boolean;
	reasons?: AgentReadinessReason[];
	configuration: AgentDelegationSetupField[];
	oauth: AgentDelegationSetupOAuthState;
}

// A value the user enters for a component. `set` says whether they set it through the agent; the value
// itself is never returned.
export interface AgentDelegationSetupField {
	key: string;
	name: string;
	description: string;
	sensitive: boolean;
	required: boolean;
	usage: string;
	options?: { name: string; value: string; description?: string }[];
	set: boolean;
}

// The body that sets (a string) or clears (null) values, by component ID and key. Keys left out stay
// as they are.
export interface AgentDelegationSetupConfiguration {
	components: Record<string, Record<string, string | null>>;
}

export interface AgentDelegationSetupOAuth {
	authURL: string;
	// The provider only redirects to a localhost callback, so authURL is Obot's local login URL, which
	// `obot mcp login` opens.
	localhostCallback?: boolean;
}
