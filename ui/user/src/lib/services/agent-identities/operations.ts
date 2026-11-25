import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import type {
	AdminAuditEventList,
	AgentAPIKey,
	AgentAPIKeyCreateRequest,
	AgentAPIKeyCreateResponse,
	AgentDelegation,
	AgentDelegationApprovalRequest,
	AgentDelegationRedirect,
	AgentDelegationSetup,
	AgentDelegationSetupConfiguration,
	AgentDelegationSetupOAuth,
	AgentDelegationSetupScope,
	AgentDelegationSetupVMCP,
	AgentIdentity,
	AgentIdentityAccess,
	AgentIdentityManifest,
	AgentVMCPReadiness
} from './types';

type ItemsResponse<T> = { items: T[] | null };

// includeDeleted also lists deleted agents, so that history keeps their names.
export async function listAgents(opts?: {
	fetch?: Fetcher;
	includeDeleted?: boolean;
	signal?: AbortSignal;
	dontLogErrors?: boolean;
}): Promise<AgentIdentity[]> {
	const response = (await doGet(
		`/agents${opts?.includeDeleted ? '?includeDeleted=true' : ''}`,
		opts
	)) as ItemsResponse<AgentIdentity>;
	return response.items ?? [];
}

export async function getAgent(id: string, opts?: { fetch?: Fetcher }): Promise<AgentIdentity> {
	return (await doGet(`/agents/${id}`, opts)) as AgentIdentity;
}

export async function createAgent(manifest: AgentIdentityManifest): Promise<AgentIdentity> {
	return (await doPost('/agents', manifest)) as AgentIdentity;
}

// The update replaces all metadata, so it takes the full manifest.
// dontLogErrors is for a form that shows the server's validation errors next to the fields they name.
export async function updateAgent(
	id: string,
	manifest: AgentIdentityManifest,
	opts?: { dontLogErrors?: boolean }
): Promise<AgentIdentity> {
	return (await doPut(`/agents/${id}`, manifest, opts)) as AgentIdentity;
}

export async function deleteAgent(id: string): Promise<void> {
	await doDelete(`/agents/${id}`);
}

export async function disableAgent(id: string): Promise<AgentIdentity> {
	return (await doPost(`/agents/${id}/disable`, {})) as AgentIdentity;
}

export async function enableAgent(id: string): Promise<AgentIdentity> {
	return (await doPost(`/agents/${id}/enable`, {})) as AgentIdentity;
}

export async function getAgentAccess(id: string): Promise<AgentIdentityAccess> {
	return (await doGet(`/agents/${id}/access`)) as AgentIdentityAccess;
}

export async function listAgentVMCPReadiness(id: string): Promise<AgentVMCPReadiness[]> {
	const response = (await doGet(`/agents/${id}/vmcps`)) as ItemsResponse<AgentVMCPReadiness>;
	return response.items ?? [];
}

export async function listAgentDelegations(id: string): Promise<AgentDelegation[]> {
	const response = (await doGet(`/agents/${id}/delegations`)) as ItemsResponse<AgentDelegation>;
	return response.items ?? [];
}

// Revoking ends the agent's access for the user, and deletes what the user set up through the agent.
export async function revokeAgentDelegation(id: string, delegationId: string): Promise<void> {
	await doDelete(`/agents/${id}/delegations/${delegationId}`);
}

export async function listAgentAPIKeys(
	id: string,
	opts?: { fetch?: Fetcher; showRevoked?: boolean }
): Promise<AgentAPIKey[]> {
	const response = (await doGet(
		`/agents/${id}/api-keys${opts?.showRevoked ? '?show_revoked=true' : ''}`,
		opts
	)) as ItemsResponse<AgentAPIKey>;
	return response.items ?? [];
}

export async function createAgentAPIKey(
	id: string,
	request: AgentAPIKeyCreateRequest
): Promise<AgentAPIKeyCreateResponse> {
	return (await doPost(`/agents/${id}/api-keys`, request)) as AgentAPIKeyCreateResponse;
}

export async function revokeAgentAPIKey(id: string, keyId: number | string): Promise<void> {
	await doDelete(`/agents/${id}/api-keys/${keyId}`);
}

// The admin change audit, newest first. It is read by admins, read-only admins and auditors.
export async function listAdminAuditEvents(params: {
	resourceKind: string;
	resourceId: string;
	limit: number;
}): Promise<AdminAuditEventList> {
	const query = new URLSearchParams({ ...params, limit: String(params.limit) });
	const response = (await doGet(`/admin-audit-events?${query}`)) as AdminAuditEventList;
	return { ...response, items: response.items ?? [] };
}

// The routes a signed-in user approves an agent's request on. Errors are left to the approval page,
// which shows the server's message in place of the consent.

export async function getAgentDelegationRequest(
	requestId: string,
	opts?: { fetch?: Fetcher }
): Promise<AgentDelegationApprovalRequest> {
	return (await doGet(`/agent-delegation-requests/${encodeURIComponent(requestId)}`, {
		...opts,
		dontLogErrors: true
	})) as AgentDelegationApprovalRequest;
}

export async function approveAgentDelegationRequest(
	requestId: string
): Promise<AgentDelegationRedirect> {
	return (await doPost(
		`/agent-delegation-requests/${encodeURIComponent(requestId)}/approve`,
		{},
		{ dontLogErrors: true }
	)) as AgentDelegationRedirect;
}

export async function cancelAgentDelegationRequest(
	requestId: string
): Promise<AgentDelegationRedirect> {
	return (await doPost(
		`/agent-delegation-requests/${encodeURIComponent(requestId)}/cancel`,
		{},
		{ dontLogErrors: true }
	)) as AgentDelegationRedirect;
}

// The signed-in user's own delegations: the agents that may act on their behalf.
export async function listMyAgentDelegations(opts?: {
	fetch?: Fetcher;
}): Promise<AgentDelegation[]> {
	const response = (await doGet('/me/agent-delegations', opts)) as ItemsResponse<AgentDelegation>;
	return response.items ?? [];
}

// Revoking ends the agent's access for the user, and deletes what they set up through it.
export async function revokeMyAgentDelegation(delegationId: string): Promise<void> {
	await doDelete(`/me/agent-delegations/${encodeURIComponent(delegationId)}`);
}

function setupPath(scope: AgentDelegationSetupScope) {
	return scope.kind === 'request'
		? `/agent-delegation-requests/${encodeURIComponent(scope.requestId)}/setup`
		: `/me/agent-delegations/${encodeURIComponent(scope.delegationId)}/setup`;
}

function setupComponentPath(scope: AgentDelegationSetupScope, vmcpId: string, componentId: string) {
	return `${setupPath(scope)}/vmcps/${encodeURIComponent(vmcpId)}/components/${encodeURIComponent(componentId)}/oauth`;
}

export async function getAgentDelegationSetup(
	scope: AgentDelegationSetupScope,
	opts?: { fetch?: Fetcher; signal?: AbortSignal }
): Promise<AgentDelegationSetup> {
	const setup = (await doGet(setupPath(scope), {
		...opts,
		dontLogErrors: true
	})) as AgentDelegationSetup;
	return { vmcps: setup.vmcps ?? [] };
}

// Sets or clears values for a vMCP through the agent, and returns the vMCP's setup.
export async function configureAgentDelegationSetup(
	scope: AgentDelegationSetupScope,
	vmcpId: string,
	configuration: AgentDelegationSetupConfiguration
): Promise<AgentDelegationSetupVMCP> {
	return (await doPut(
		`${setupPath(scope)}/vmcps/${encodeURIComponent(vmcpId)}/configuration`,
		configuration,
		{ dontLogErrors: true }
	)) as AgentDelegationSetupVMCP;
}

// Starts connecting the user's upstream account for a component through the agent. The provider's
// callback returns the browser to the page that started it.
export async function startAgentDelegationSetupOAuth(
	scope: AgentDelegationSetupScope,
	vmcpId: string,
	componentId: string
): Promise<AgentDelegationSetupOAuth> {
	return (await doPost(
		setupComponentPath(scope, vmcpId, componentId),
		{},
		{ dontLogErrors: true }
	)) as AgentDelegationSetupOAuth;
}

// Disconnects the user's upstream account for a component through the agent, and returns the vMCP's
// setup.
export async function disconnectAgentDelegationSetupOAuth(
	scope: AgentDelegationSetupScope,
	vmcpId: string,
	componentId: string
): Promise<AgentDelegationSetupVMCP> {
	return (await doDelete(setupComponentPath(scope, vmcpId, componentId), {
		dontLogErrors: true
	})) as AgentDelegationSetupVMCP;
}
