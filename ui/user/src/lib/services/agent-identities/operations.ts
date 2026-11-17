import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import type {
	AdminAuditEventList,
	AgentAPIKey,
	AgentAPIKeyCreateRequest,
	AgentAPIKeyCreateResponse,
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
export async function updateAgent(
	id: string,
	manifest: AgentIdentityManifest
): Promise<AgentIdentity> {
	return (await doPut(`/agents/${id}`, manifest)) as AgentIdentity;
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

// The readiness of one vMCP, which also lists the profiles that supply the agent's values and grants.
export async function getAgentVMCPReadiness(
	id: string,
	vmcpID: string,
	opts?: { fetch?: Fetcher }
): Promise<AgentVMCPReadiness> {
	return (await doGet(`/agents/${id}/vmcps/${vmcpID}/readiness`, opts)) as AgentVMCPReadiness;
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
