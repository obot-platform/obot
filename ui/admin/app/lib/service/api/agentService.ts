import {
	Agent,
	AgentAuthorization,
	CreateAgent,
	UpdateAgent,
} from "~/lib/model/agents";
import { EntityList } from "~/lib/model/primitives";
import { WorkspaceFile } from "~/lib/model/workspace";
import {
	ApiRoutes,
	createRevalidate,
	revalidateWhere,
} from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { downloadUrl } from "~/lib/utils/downloadFile";

async function getAgents() {
	const res = await request<{ items: Agent[] }>({
		url: ApiRoutes.agents.base().url,
		errorMessage: "Failed to fetch agents",
	});

	return res.data.items ?? ([] as Agent[]);
}
getAgents.key = () => ({ url: ApiRoutes.agents.base().path }) as const;

const getAgentById = async (agentId: string) => {
	const res = await request<Agent>({
		url: ApiRoutes.agents.getById(agentId).url,
		errorMessage: "Failed to fetch agent",
	});

	if (!res.data) return null;

	return res.data;
};
getAgentById.key = (agentId?: Nullish<string>) => {
	if (!agentId) return null;

	return { url: ApiRoutes.agents.getById(agentId).path, agentId };
};

async function createAgent({ agent }: { agent: CreateAgent }) {
	const res = await request<Agent>({
		url: ApiRoutes.agents.base().url,
		method: "POST",
		data: agent,
		errorMessage: "Failed to create agent",
	});

	return res.data;
}

async function updateAgent({ id, agent }: { id: string; agent: UpdateAgent }) {
	const res = await request<Agent>({
		url: ApiRoutes.agents.getById(id).url,
		method: "PUT",
		data: agent,
		errorMessage: "Failed to update agent",
	});

	return res.data;
}

async function deleteAgent(id: string) {
	await request({
		url: ApiRoutes.agents.getById(id).url,
		method: "DELETE",
		errorMessage: "Failed to delete agent",
	});
}

async function getAuthUrlForAgent(agentId: string, toolRef: string) {
	const res = await request<Agent>({
		url: ApiRoutes.agents.getAuthUrlForAgent(agentId, toolRef).url,
		errorMessage: "Failed to get auth url for agent",
		method: "POST",
	});

	return res.data.authStatus?.[toolRef];
}

const revalidateAgents = () =>
	revalidateWhere((url) => url.includes(ApiRoutes.agents.base().path));

async function getAgentAuthorizations(agentId: string) {
	const res = await request<{ items: AgentAuthorization[] }>({
		url: ApiRoutes.agents.getAuthorizations(agentId).url,
		errorMessage: "Failed to fetch agent authorizations",
	});

	return res.data.items;
}
getAgentAuthorizations.key = (agentId?: Nullish<string>) => {
	if (!agentId) return null;

	return { url: ApiRoutes.agents.getAuthorizations(agentId).path, agentId };
};

async function addAgentAuthorization(agentId: string, userId: string) {
	await request({
		url: ApiRoutes.agents.addAuthorization(agentId).url,
		method: "POST",
		data: { userId },
		errorMessage: "Failed to add agent authorization",
	});
}

async function removeAgentAuthorization(agentId: string, userId: string) {
	await request({
		url: ApiRoutes.agents.removeAuthorization(agentId).url,
		method: "POST",
		data: { userId },
		errorMessage: "Failed to remove agent authorization",
	});
}

async function getWorkspaceFiles(agentId: string) {
	const res = await request<EntityList<WorkspaceFile>>({
		url: ApiRoutes.agents.getWorkspaceFiles(agentId).url,
		errorMessage: "Failed to fetch workspace files",
	});

	return res.data.items;
}
getWorkspaceFiles.key = (agentId?: Nullish<string>) => {
	if (!agentId) return null;

	return { url: ApiRoutes.agents.getWorkspaceFiles(agentId).path, agentId };
};
getWorkspaceFiles.revalidate = createRevalidate(
	ApiRoutes.agents.getWorkspaceFiles
);

async function uploadWorkspaceFile(agentId: string, file: File) {
	await request({
		url: ApiRoutes.agents.uploadWorkspaceFile(agentId, file.name).url,
		method: "POST",
		data: await file.arrayBuffer(),
		headers: { "Content-Type": "application/x-www-form-urlencoded" },
		errorMessage: "Failed to add knowledge to agent",
	});

	return file.name;
}

async function deleteWorkspaceFile(agentId: string, fileName: string) {
	await request({
		url: ApiRoutes.agents.removeWorkspaceFile(agentId, fileName).url,
		method: "DELETE",
		errorMessage: "Failed to delete workspace file",
	});

	return fileName;
}

async function downloadWorkspaceFile(agentId: string, fileName: string) {
	downloadUrl(
		ApiRoutes.agents.getWorkspaceFile(agentId, fileName).url,
		fileName
	);
}

export const AgentService = {
	getAgents,
	getAgentById,
	createAgent,
	updateAgent,
	deleteAgent,
	getAuthUrlForAgent,
	revalidateAgents,
	getAgentAuthorizations,
	addAgentAuthorization,
	removeAgentAuthorization,
	getWorkspaceFiles,
	uploadWorkspaceFile,
	deleteWorkspaceFile,
	downloadWorkspaceFile,
};
