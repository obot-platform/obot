import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import type {
	ProjectV2,
	ProjectV2Agent,
	ProjectV2AgentCreateRequest,
	ProjectV2AgentUpdateRequest,
	ProjectV2CreateRequest,
	ProjectV2UpdateRequest
} from './types';

type ItemsResponse<T> = { items: T[] | null };

export async function listProjectsV2(opts?: { fetch?: Fetcher }): Promise<ProjectV2[]> {
	const response = (await doGet('/projectsv2', opts)) as ItemsResponse<ProjectV2>;
	return response.items ?? [];
}

export async function getProjectV2(id: string, opts?: { fetch?: Fetcher }): Promise<ProjectV2> {
	const response = (await doGet(`/projectsv2/${id}`, opts)) as ProjectV2;
	return response;
}

export async function createProjectV2(
	request: ProjectV2CreateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2> {
	const response = (await doPost('/projectsv2', request, opts)) as ProjectV2;
	return response;
}

export async function updateProjectV2(
	id: string,
	request: ProjectV2UpdateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2> {
	const response = (await doPut(`/projectsv2/${id}`, request, opts)) as ProjectV2;
	return response;
}

export async function deleteProjectV2(id: string): Promise<void> {
	await doDelete(`/projectsv2/${id}`);
}

export async function listProjectV2Agents(
	projectId: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent[]> {
	const response = (await doGet(
		`/projectsv2/${projectId}/agents`,
		opts
	)) as ItemsResponse<ProjectV2Agent>;
	return response.items ?? [];
}

export async function getProjectV2Agent(
	projectId: string,
	agentId: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doGet(
		`/projectsv2/${projectId}/agents/${agentId}`,
		opts
	)) as ProjectV2Agent;
	return response;
}

export async function createProjectV2Agent(
	projectId: string,
	request: ProjectV2AgentCreateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doPost(
		`/projectsv2/${projectId}/agents`,
		request,
		opts
	)) as ProjectV2Agent;
	return response;
}

export async function updateProjectV2Agent(
	projectId: string,
	agentId: string,
	request: ProjectV2AgentUpdateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doPut(
		`/projectsv2/${projectId}/agents/${agentId}`,
		request,
		opts
	)) as ProjectV2Agent;
	return response;
}

export async function deleteProjectV2Agent(projectId: string, agentId: string): Promise<void> {
	await doDelete(`/projectsv2/${projectId}/agents/${agentId}`);
}

export async function launchProjectV2Agent(
	projectId: string,
	agentId: string,
	opts?: { fetch?: Fetcher }
): Promise<unknown> {
	const response = (await doPost(
		`/projectsv2/${projectId}/agents/${agentId}/launch`,
		{},
		opts
	)) as unknown;
	return response;
}
