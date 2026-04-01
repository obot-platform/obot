import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import type {
	ProjectV2,
	ProjectV2Agent,
	ProjectV2AgentCreateRequest,
	ProjectV2AgentUpdateRequest,
	ProjectV2CreateRequest,
	ProjectV2UpdateRequest,
	PublishedArtifact,
	PublishedArtifactUpdateRequest,
	Skill
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

export async function listAllNanobotAgents(opts?: { fetch?: Fetcher }): Promise<ProjectV2Agent[]> {
	const response = (await doGet('/nanobot-agents', opts)) as ItemsResponse<ProjectV2Agent>;
	return response.items ?? [];
}

export async function listPublishedWorkflows(opts?: {
	fetch?: Fetcher;
}): Promise<PublishedArtifact[]> {
	const response = (await doGet(
		`/published-artifacts?type=workflow`,
		opts
	)) as ItemsResponse<PublishedArtifact>;
	return response.items ?? [];
}

export async function getPublishedArtifact(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<PublishedArtifact> {
	const response = (await doGet(`/published-artifacts/${id}`, opts)) as PublishedArtifact;
	return response;
}

export async function updatePublishedArtifact(
	id: string,
	request: PublishedArtifactUpdateRequest,
	opts?: { fetch?: Fetcher }
): Promise<PublishedArtifact> {
	const response = (await doPut(`/published-artifacts/${id}`, request, opts)) as PublishedArtifact;
	return response;
}

export async function deletePublishedArtifact(id: string): Promise<void> {
	await doDelete(`/published-artifacts/${id}`);
}

export async function listSkills(opts?: {
	fetch?: Fetcher;
	query?: string;
	limit?: number;
}): Promise<Skill[]> {
	const params = new URLSearchParams();
	if (opts?.query != null) params.set('q', opts.query);
	if (opts?.limit != null) params.set('limit', opts.limit.toString());
	const queryString = params.toString();
	const url = queryString ? `/skills?${queryString}` : '/skills';
	const response = (await doGet(url, opts)) as ItemsResponse<Skill>;
	return response.items ?? [];
}

export async function getSkill(id: string, opts?: { fetch?: Fetcher }): Promise<Skill> {
	const response = (await doGet(`/skills/${id}`, opts)) as Skill;
	return response;
}

export async function getPublishedArtifactVersionContents(
	id: string,
	version: number,
	opts?: { fetch?: Fetcher }
): Promise<string> {
	const response = (await doGet(`/published-artifacts/${id}/${version}/skill`, {
		...opts,
		text: true
	})) as string;
	return response;
}
