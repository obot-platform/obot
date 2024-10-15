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
        errorMessage: "Failed to fetch knowledge for agent",
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
        errorMessage: "Failed to add knowledge to agent",
    });
}

async function deleteKnowledgeFromAgent(agentId: string, fileName: string) {
    await request({
        url: ApiRoutes.agents.deleteKnowledge(agentId, fileName).url,
        method: "DELETE",
        errorMessage: "Failed to delete knowledge from agent",
    });
}

async function triggerKnowledgeIngestion(agentId: string) {
    await request({
        url: ApiRoutes.agents.triggerKnowledgeIngestion(agentId).url,
        method: "POST",
        errorMessage: "Failed to trigger knowledge ingestion",
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
        errorMessage: "Failed to create remote knowledge source",
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
        errorMessage: "Failed to update remote knowledge source",
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
        errorMessage: "Failed to resync remote knowledge source",
    });
}

async function approveKnowledgeFile(
    agentId: string,
    fileID: string,
    approve: boolean
) {
    await request({
        url: ApiRoutes.agents.approveKnowledgeFile(agentId, fileID).url,
        method: "PUT",
        data: JSON.stringify({ approve }),
        errorMessage: "Failed to approve knowledge file",
    });
}

async function getRemoteKnowledgeSource(agentId: string) {
    const res = await request<{
        items: RemoteKnowledgeSource[];
    }>({
        url: ApiRoutes.agents.getRemoteKnowledgeSource(agentId).url,
        errorMessage: "Failed to fetch remote knowledge source",
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
    approveKnowledgeFile,
    getKnowledgeForAgent,
    addKnowledgeToAgent,
    deleteKnowledgeFromAgent,
    triggerKnowledgeIngestion,
    createRemoteKnowledgeSource,
    updateRemoteKnowledgeSource,
    resyncRemoteKnowledgeSource,
    getRemoteKnowledgeSource,
};
