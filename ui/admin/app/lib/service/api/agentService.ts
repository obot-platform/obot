import { Agent, CreateAgent, UpdateAgent } from "~/lib/model/agents";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

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

export const AgentService = {
    getAgents,
    getAgentById,
    createAgent,
    updateAgent,
    deleteAgent,
    getAuthUrlForAgent,
    revalidateAgents,
};
