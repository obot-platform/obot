import { z } from "zod";

import {
	Agent,
	AgentAuthorization,
	CreateAgent,
	UpdateAgent,
} from "~/lib/model/agents";
import { EntityList } from "~/lib/model/primitives";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import {
	createFetcher,
	createMutator,
} from "~/lib/service/api/service-primitives";
import { downloadUrl } from "~/lib/utils/downloadFile";

const getAgents = createFetcher(
	z.object({}),
	async (_, { signal }) => {
		const res = await request<{ items: Agent[] }>({
			url: ApiRoutes.agents.base().url,
			errorMessage: "Failed to fetch agents",
			signal,
		});

		return res.data.items ?? ([] as Agent[]);
	},
	() => ApiRoutes.agents.base().path
);

const getAgentById = createFetcher(
	z.object({ agentId: z.string() }),
	async ({ agentId }, { signal }) => {
		const res = await request<Agent>({
			url: ApiRoutes.agents.getById(agentId).url,
			errorMessage: "Failed to fetch agent",
			signal,
		});

		return res.data;
	},
	() => ApiRoutes.agents.getById(":agentId").path
);

const createAgent = createMutator(
	async ({ agent }: { agent: CreateAgent }, { signal }) => {
		const res = await request<Agent>({
			url: ApiRoutes.agents.base().url,
			method: "POST",
			data: agent,
			errorMessage: "Failed to create agent",
			signal,
		});

		return res.data;
	}
);

const updateAgent = createMutator(
	async ({ id, agent }: { id: string; agent: UpdateAgent }, { signal }) => {
		const res = await request<Agent>({
			url: ApiRoutes.agents.getById(id).url,
			method: "PUT",
			data: agent,
			errorMessage: "Failed to update agent",
			signal,
		});

		return res.data;
	}
);

const deleteAgent = createMutator(
	async ({ id }: { id: string }, { signal }) => {
		await request({
			url: ApiRoutes.agents.getById(id).url,
			method: "DELETE",
			errorMessage: "Failed to delete agent",
			signal,
		});
	}
);

const getAuthUrlForAgent = createFetcher(
	z.object({ agentId: z.string(), toolRef: z.string() }),
	async ({ agentId, toolRef }, { signal }) => {
		const res = await request<Agent>({
			url: ApiRoutes.agents.getAuthUrlForAgent(agentId, toolRef).url,
			errorMessage: "Failed to get auth url for agent",
			method: "POST",
			signal,
		});

		return res.data.authStatus?.[toolRef];
	},
	() => ApiRoutes.agents.getAuthUrlForAgent(":agentId", ":toolRef").path
);

const getAgentAuthorizations = createFetcher(
	z.object({ agentId: z.string() }),
	async ({ agentId }, { signal }) => {
		const res = await request<{ items: AgentAuthorization[] }>({
			url: ApiRoutes.agents.getAuthorizations(agentId).url,
			errorMessage: "Failed to fetch agent authorizations",
			signal,
		});

		return res.data.items;
	},
	() => ApiRoutes.agents.getAuthorizations(":agentId").path
);

type AddAgentAuthorizationParams = {
	agentId: string;
	userId: string;
};

const addAgentAuthorization = createMutator(
	async ({ agentId, userId }: AddAgentAuthorizationParams, { signal }) => {
		await request({
			url: ApiRoutes.agents.addAuthorization(agentId).url,
			method: "POST",
			data: { userId },
			errorMessage: "Failed to add agent authorization",
			signal,
		});
	}
);

type RemoveAgentAuthorizationParams = {
	agentId: string;
	userId: string;
};

const removeAgentAuthorization = createMutator(
	async ({ agentId, userId }: RemoveAgentAuthorizationParams, { signal }) => {
		await request({
			url: ApiRoutes.agents.removeAuthorization(agentId).url,
			method: "POST",
			data: { userId },
			errorMessage: "Failed to remove agent authorization",
			signal,
		});
	}
);

const getWorkspaceFiles = createFetcher(
	z.object({ agentId: z.string() }),
	async ({ agentId }, { signal }) => {
		const res = await request<EntityList<WorkspaceFile>>({
			url: ApiRoutes.agents.getWorkspaceFiles(agentId).url,
			errorMessage: "Failed to fetch workspace files",
			signal,
		});

		return res.data.items;
	},
	() => ApiRoutes.agents.getWorkspaceFiles(":agentId").path
);

type UploadWorkspaceFileParams = {
	agentId: string;
	file: File;
};

const uploadWorkspaceFile = createMutator(
	async ({ agentId, file }: UploadWorkspaceFileParams, { signal }) => {
		await request({
			url: ApiRoutes.agents.uploadWorkspaceFile(agentId, file.name).url,
			method: "POST",
			data: await file.arrayBuffer(),
			headers: { "Content-Type": "application/x-www-form-urlencoded" },
			errorMessage: "Failed to add knowledge to agent",
			signal,
		});

		return file.name;
	}
);

type DeleteWorkspaceFileParams = {
	agentId: string;
	fileName: string;
};

const deleteWorkspaceFile = createMutator(
	async ({ agentId, fileName }: DeleteWorkspaceFileParams, { signal }) => {
		await request({
			url: ApiRoutes.agents.removeWorkspaceFile(agentId, fileName).url,
			method: "DELETE",
			errorMessage: "Failed to delete workspace file",
			signal,
		});

		return fileName;
	}
);

async function downloadWorkspaceFile(agentId: string, fileName: string) {
	downloadUrl(
		ApiRoutes.agents.getWorkspaceFile(agentId, fileName).url,
		fileName
	);
}

const setDefaultAgent = createMutator(
	async ({ agentId }: { agentId: string }, { signal }) => {
		await request({
			url: ApiRoutes.agents.setDefault(agentId).url,
			method: "PUT",
			errorMessage: "Failed to set default agent",
			signal,
		});
	}
);

export const AgentService = {
	getAgents: getAgents,
	getAgentById: getAgentById,
	createAgent: createAgent,
	updateAgent: updateAgent,
	deleteAgent: deleteAgent,
	getAuthUrlForAgent: getAuthUrlForAgent,
	getAgentAuthorizations: getAgentAuthorizations,
	addAgentAuthorization: addAgentAuthorization,
	removeAgentAuthorization: removeAgentAuthorization,
	getWorkspaceFiles: getWorkspaceFiles,
	uploadWorkspaceFile: uploadWorkspaceFile,
	deleteWorkspaceFile: deleteWorkspaceFile,
	downloadWorkspaceFile: downloadWorkspaceFile,
	setDefaultAgent,
};
