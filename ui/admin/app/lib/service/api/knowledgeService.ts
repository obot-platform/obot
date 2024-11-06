import {
    KnowledgeFile,
    KnowledgeSource,
    KnowledgeSourceInput,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getLocalKnowledgeFilesForAgent(agentId: string) {
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.agents.getLocalKnowledgeFiles(agentId).url,
        errorMessage: "Failed to fetch knowledge for agent",
    });

    return res.data.items;
}
getLocalKnowledgeFilesForAgent.key = (agentId?: Nullish<string>) => {
    if (!agentId) return null;

    return {
        url: ApiRoutes.agents.getLocalKnowledgeFiles(agentId).path,
        agentId,
    };
};

async function addKnowledgeFilesToAgent(agentId: string, file: File) {
    const res = await request<KnowledgeFile>({
        url: ApiRoutes.agents.addKnowledgeFiles(agentId, file.name).url,
        method: "POST",
        data: await file.arrayBuffer(),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        errorMessage: "Failed to add knowledge to agent",
    });
    return res.data;
}

async function deleteKnowledgeFileFromAgent(agentId: string, fileName: string) {
    await request({
        url: ApiRoutes.agents.deleteKnowledgeFiles(agentId, fileName).url,
        method: "DELETE",
        errorMessage: "Failed to delete knowledge from agent",
    });
}

async function createKnowledgeSource(
    agentId: string,
    input: KnowledgeSourceInput
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.agents.createKnowledgeSource(agentId).url,
        method: "POST",
        data: JSON.stringify(input),
        errorMessage: "Failed to create remote knowledge source",
    });
    return res.data;
}

async function updateKnowledgeSource(
    agentId: string,
    knowledgeSourceId: string,
    input: KnowledgeSourceInput
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.agents.updateKnowledgeSource(agentId, knowledgeSourceId)
            .url,
        method: "PUT",
        data: JSON.stringify(input),
        errorMessage: "Failed to update remote knowledge source",
    });
    return res.data;
}

async function resyncKnowledgeSource(
    agentId: string,
    knowledgeSourceId: string
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.agents.syncKnowledgeSource(agentId, knowledgeSourceId)
            .url,
        method: "POST",
        errorMessage: "Failed to resync remote knowledge source",
    });
    return res.data;
}

async function approveFile(agentId: string, fileID: string, approve: boolean) {
    const res = await request<KnowledgeFile>({
        url: ApiRoutes.agents.approveFile(agentId, fileID).url,
        method: "POST",
        data: JSON.stringify({ Approved: approve }),
        errorMessage: "Failed to approve knowledge file",
    });
    return res.data;
}

async function getKnowledgeSourcesForAgent(agentId: string) {
    const res = await request<{
        items: KnowledgeSource[];
    }>({
        url: ApiRoutes.agents.getKnowledgeSource(agentId).url,
        errorMessage: "Failed to fetch remote knowledge source",
    });
    return res.data.items;
}

getKnowledgeSourcesForAgent.key = (agentId?: Nullish<string>) => {
    if (!agentId) return null;

    return {
        url: ApiRoutes.agents.getKnowledgeSource(agentId).path,
        agentId,
    };
};

async function getFilesForKnowledgeSource(agentId: string, sourceId: string) {
    if (!sourceId) return [];
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.agents.getFilesForKnowledgeSource(agentId, sourceId).url,
        errorMessage: "Failed to fetch knowledge files for knowledgesource",
    });
    return res.data.items;
}

getFilesForKnowledgeSource.key = (
    agentId?: Nullish<string>,
    sourceId?: Nullish<string>
) => {
    if (!agentId || !sourceId) return null;

    return {
        url: ApiRoutes.agents.getFilesForKnowledgeSource(agentId, sourceId)
            .path,
        agentId,
        sourceId,
    };
};

async function reingestFile(
    agentId: string,
    fileID: string,
    sourceId?: string
) {
    const rest = await request<KnowledgeFile>({
        url: ApiRoutes.agents.reingestFile(agentId, fileID, sourceId).url,
        method: "POST",
        errorMessage: "Failed to reingest knowledge file",
    });
    return rest.data;
}

async function deleteKnowledgeSource(agentId: string, sourceId: string) {
    await request({
        url: ApiRoutes.agents.deleteKnowledgeSource(agentId, sourceId).url,
        method: "DELETE",
        errorMessage: "Failed to delete knowledge source",
    });
}

export const KnowledgeService = {
    approveFile,
    getLocalKnowledgeFilesForAgent,
    addKnowledgeFilesToAgent,
    deleteKnowledgeFileFromAgent,
    createKnowledgeSource,
    updateKnowledgeSource,
    resyncKnowledgeSource,
    getKnowledgeSourcesForAgent,
    getFilesForKnowledgeSource,
    reingestFile,
    deleteKnowledgeSource,
};
