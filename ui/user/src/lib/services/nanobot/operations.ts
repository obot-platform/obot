import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import type {
	Project,
	ProjectCreateRequest,
	ProjectUpdateRequest,
	ProjectV2Agent,
	ProjectV2AgentCreateRequest,
	ProjectV2AgentUpdateRequest,
	PublishedArtifact,
	PublishedArtifactUpdateRequest,
	Skill
} from './types';

type ItemsResponse<T> = { items: T[] | null };

export async function listProjects(opts?: { fetch?: Fetcher }): Promise<Project[]> {
	const response = (await doGet('/projects', opts)) as ItemsResponse<Project>;
	return response.items ?? [];
}

export async function getProject(id: string, opts?: { fetch?: Fetcher }): Promise<Project> {
	const response = (await doGet(`/projects/${id}`, opts)) as Project;
	return response;
}

export async function createProject(
	request: ProjectCreateRequest,
	opts?: { fetch?: Fetcher }
): Promise<Project> {
	const response = (await doPost('/projects', request, opts)) as Project;
	return response;
}

export async function updateProject(
	id: string,
	request: ProjectUpdateRequest,
	opts?: { fetch?: Fetcher }
): Promise<Project> {
	const response = (await doPut(`/projects/${id}`, request, opts)) as Project;
	return response;
}

export async function deleteProject(id: string): Promise<void> {
	await doDelete(`/projects/${id}`);
}

export async function listProjectAgents(
	projectId: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent[]> {
	const response = (await doGet(
		`/projects/${projectId}/agents`,
		opts
	)) as ItemsResponse<ProjectV2Agent>;
	return response.items ?? [];
}

export async function getProjectAgent(
	projectId: string,
	agentId: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doGet(
		`/projects/${projectId}/agents/${agentId}`,
		opts
	)) as ProjectV2Agent;
	return response;
}

export async function createProjectAgent(
	projectId: string,
	request: ProjectV2AgentCreateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doPost(`/projects/${projectId}/agents`, request, opts)) as ProjectV2Agent;
	return response;
}

export async function updateProjectAgent(
	projectId: string,
	agentId: string,
	request: ProjectV2AgentUpdateRequest,
	opts?: { fetch?: Fetcher }
): Promise<ProjectV2Agent> {
	const response = (await doPut(
		`/projects/${projectId}/agents/${agentId}`,
		request,
		opts
	)) as ProjectV2Agent;
	return response;
}

export async function deleteProjectAgent(projectId: string, agentId: string): Promise<void> {
	await doDelete(`/projects/${projectId}/agents/${agentId}`);
}

export async function launchProjectAgent(
	projectId: string,
	agentId: string,
	opts?: { fetch?: Fetcher }
): Promise<unknown> {
	const response = (await doPost(
		`/projects/${projectId}/agents/${agentId}/launch`,
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
	dontLogErrors?: boolean;
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
