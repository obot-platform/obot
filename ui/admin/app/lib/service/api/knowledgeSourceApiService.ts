import {
    KnowledgeFile,
    KnowledgeNamespace,
    KnowledgeSource,
    KnowledgeSourceInput,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function createKnowledgeSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    input: KnowledgeSourceInput
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.knowledgeSources.createKnowledgeSource(
            namespace,
            agentId
        ).url,
        method: "POST",
        data: JSON.stringify(input),
        errorMessage: "Failed to create remote knowledge source",
    });
    return res.data;
}

async function updateKnowledgeSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    knowledgeSourceId: string,
    input: KnowledgeSourceInput
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.knowledgeSources.updateKnowledgeSource(
            namespace,
            agentId,
            knowledgeSourceId
        ).url,
        method: "PUT",
        data: JSON.stringify(input),
        errorMessage: "Failed to update remote knowledge source",
    });
    return res.data;
}

async function resyncKnowledgeSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    knowledgeSourceId: string
) {
    const res = await request<KnowledgeSource>({
        url: ApiRoutes.knowledgeSources.syncKnowledgeSource(
            namespace,
            agentId,
            knowledgeSourceId
        ).url,
        method: "POST",
        errorMessage: "Failed to resync remote knowledge source",
    });
    return res.data;
}

async function approveFile(
    namespace: KnowledgeNamespace,
    agentId: string,
    fileID: string,
    approve: boolean
) {
    const res = await request<KnowledgeFile>({
        url: ApiRoutes.knowledgeSources.approveFile(namespace, agentId, fileID)
            .url,
        method: "POST",
        data: JSON.stringify({ Approved: approve }),
        errorMessage: "Failed to approve knowledge file",
    });
    return res.data;
}

async function getKnowledgeSources(
    namespace: KnowledgeNamespace,
    agentId: string
) {
    const res = await request<{
        items: KnowledgeSource[];
    }>({
        url: ApiRoutes.knowledgeSources.getKnowledgeSources(namespace, agentId)
            .url,
        errorMessage: "Failed to fetch remote knowledge source",
    });
    return res.data.items;
}
getKnowledgeSources.key = (
    namespace?: Nullish<KnowledgeNamespace>,
    agentId?: Nullish<string>
) => {
    if (!namespace || !agentId) return null;

    return {
        url: ApiRoutes.knowledgeSources.getKnowledgeSources(namespace, agentId)
            .path,
        agentId,
        namespace,
    };
};

async function getFilesForKnowledgeSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    sourceId: string
) {
    if (!sourceId) return [];
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.knowledgeSources.getFilesForKnowledgeSource(
            namespace,
            agentId,
            sourceId
        ).url,
        errorMessage: "Failed to fetch knowledge files for knowledgesource",
    });
    return res.data.items;
}

getFilesForKnowledgeSource.key = (
    namespace?: Nullish<KnowledgeNamespace>,
    agentId?: Nullish<string>,
    sourceId?: Nullish<string>
) => {
    if (!namespace || !agentId || !sourceId) return null;

    return {
        url: ApiRoutes.knowledgeSources.getFilesForKnowledgeSource(
            namespace,
            agentId,
            sourceId
        ).path,
        agentId,
        sourceId,
    };
};

async function reingestFileFromSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    sourceId: string,
    fileID: string
) {
    const { url } = ApiRoutes.knowledgeSources.reingestKnowledgeFileFromSource(
        namespace,
        agentId,
        sourceId,
        fileID
    );

    const res = await request<KnowledgeFile>({
        url,
        method: "POST",
        errorMessage: "Failed to reingest knowledge file from source",
    });

    return res.data;
}

async function deleteKnowledgeSource(
    namespace: KnowledgeNamespace,
    agentId: string,
    sourceId: string
) {
    await request({
        url: ApiRoutes.knowledgeSources.deleteKnowledgeSource(
            namespace,
            agentId,
            sourceId
        ).url,
        method: "DELETE",
        errorMessage: "Failed to delete knowledge source",
    });
}

export const KnowledgeSourceApiService = {
    approveFile,
    createKnowledgeSource,
    updateKnowledgeSource,
    resyncKnowledgeSource,
    getKnowledgeSources,
    getFilesForKnowledgeSource,
    reingestFileFromSource,
    deleteKnowledgeSource,
};
