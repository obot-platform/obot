import { ChatEvent } from "~/lib/model/chatEvents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { Thread } from "~/lib/model/threads";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getThreads = async () => {
    const res = await request<{ items: Thread[] }>({
        url: ApiRoutes.threads.base().url,
    });

    return res.data.items ?? ([] as Thread[]);
};
getThreads.key = () => ({ url: ApiRoutes.threads.base().path }) as const;

const getThreadById = async (threadId: string) => {
    const res = await request<Thread>({
        url: ApiRoutes.threads.getById(threadId).url,
    });

    return res.data;
};
getThreadById.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.threads.getById(threadId).path, threadId };
};

const getThreadsByAgent = async (agentId: string) => {
    const res = await request<{ items: Thread[] }>({
        url: ApiRoutes.threads.getByAgent(agentId).url,
    });

    return res.data.items ?? ([] as Thread[]);
};
getThreadsByAgent.key = (agentId?: Nullish<string>) => {
    if (!agentId) return null;

    return { url: ApiRoutes.threads.getByAgent(agentId).path, agentId };
};

const getThreadEvents = async (threadId: string) => {
    const res = await request<{ items: ChatEvent[] }>({
        url: ApiRoutes.threads.events(threadId).url,
        headers: { Accept: "application/json" },
    });

    return res.data.items ?? ([] as ChatEvent[]);
};
getThreadEvents.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.threads.events(threadId).path, threadId };
};

const deleteThread = async (threadId: string) => {
    await request({
        url: ApiRoutes.threads.getById(threadId).url,
        method: "DELETE",
    });
};

const getKnowledge = async (threadId: string) => {
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.threads.getKnowledge(threadId).url,
    });

    return res.data.items ?? ([] as KnowledgeFile[]);
};
getKnowledge.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.threads.getKnowledge(threadId).path, threadId };
};

const getFiles = async (threadId: string) => {
    const res = await request<{ items: WorkspaceFile[] }>({
        url: ApiRoutes.threads.getFiles(threadId).url,
    });

    return res.data.items ?? ([] as WorkspaceFile[]);
};
getFiles.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.threads.getFiles(threadId).path, threadId };
};

const revalidateThreads = () =>
    revalidateWhere((url) => url.includes(ApiRoutes.threads.base().path));

export const ThreadsService = {
    getThreads,
    getThreadById,
    getThreadsByAgent,
    getThreadEvents,
    deleteThread,
    revalidateThreads,
    getKnowledge,
    getFiles,
};
