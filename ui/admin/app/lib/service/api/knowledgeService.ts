import {
    KnowledgeFile,
    RemoteKnowledgeSource,
    RemoteKnowledgeSourceInput,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getKnowledgeForAgent(agentId: string, includeDeleted = false) {
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.agents.getKnowledge(agentId).url,
    });

    if (includeDeleted) return res.data.items;

    // filter out deleted files
    return res.data.items.filter((item) => !item.deleted);
}
getKnowledgeForAgent.key = (agentId?: Nullish<string>) => {
    if (!agentId) return null;

    return { url: ApiRoutes.agents.getKnowledge(agentId).path, agentId };
};

async function addKnowledgeToAgent(agentId: string, file: File) {
    await request({
        url: ApiRoutes.agents.addKnowledge(agentId, file.name).url,
        method: "POST",
        data: await file.arrayBuffer(),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
}

async function deleteKnowledgeFromAgent(agentId: string, fileName: string) {
    await request({
        url: ApiRoutes.agents.deleteKnowledge(agentId, fileName).url,
        method: "DELETE",
    });
}

async function triggerKnowledgeIngestion(agentId: string) {
    await request({
        url: ApiRoutes.agents.triggerKnowledgeIngestion(agentId).url,
        method: "POST",
    });
}

async function createRemoteKnowledgeSource(
    agentId: string,
    input: RemoteKnowledgeSourceInput
) {
    const res = await request<RemoteKnowledgeSource>({
        url: ApiRoutes.agents.createRemoteKnowledgeSource(agentId).url,
        method: "POST",
        data: JSON.stringify(input),
    });
    return res.data;
}

async function updateRemoteKnowledgeSource(
    agentId: string,
    remoteKnowledgeSourceId: string,
    input: RemoteKnowledgeSourceInput
) {
    await request({
        url: ApiRoutes.agents.updateRemoteKnowledgeSource(
            agentId,
            remoteKnowledgeSourceId
        ).url,
        method: "PUT",
        data: JSON.stringify(input),
    });
}

async function resyncRemoteKnowledgeSource(
    agentId: string,
    remoteKnowledgeSourceId: string
) {
    await request({
        url: ApiRoutes.agents.updateRemoteKnowledgeSource(
            agentId,
            remoteKnowledgeSourceId
        ).url,
        method: "PATCH",
    });
}

async function getRemoteKnowledgeSource(agentId: string) {
    const res = await request<{
        items: RemoteKnowledgeSource[];
    }>({
        url: ApiRoutes.agents.getRemoteKnowledgeSource(agentId).url,
    });
    return res.data.items;
}

getRemoteKnowledgeSource.key = (agentId?: Nullish<string>) => {
    if (!agentId) return null;

    return {
        url: ApiRoutes.agents.getRemoteKnowledgeSource(agentId).path,
        agentId,
    };
};

export const KnowledgeService = {
    getKnowledgeForAgent,
    addKnowledgeToAgent,
    deleteKnowledgeFromAgent,
    triggerKnowledgeIngestion,
    createRemoteKnowledgeSource,
    updateRemoteKnowledgeSource,
    resyncRemoteKnowledgeSource,
    getRemoteKnowledgeSource,
};
