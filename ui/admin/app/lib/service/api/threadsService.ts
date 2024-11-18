import { ChatEvent } from "~/lib/model/chatEvents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { Thread } from "~/lib/model/threads";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getThreads = async () => {
    const res = await request<{ items: Thread[] }>({
        url: ApiRoutes.threads.base().url,
        errorMessage: "Failed to fetch threads",
    });

    return res.data.items ?? ([] as Thread[]);
};
getThreads.key = () => ({ url: ApiRoutes.threads.base().path }) as const;

const getThreadById = async (threadId: string) => {
    const res = await request<Thread>({
        url: ApiRoutes.threads.getById(threadId).url,
        errorMessage: "Failed to fetch thread",
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
        errorMessage: "Failed to fetch threads by agent",
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
        errorMessage: "Failed to fetch thread events",
    });

    return res.data.items ?? ([] as ChatEvent[]);
};
getThreadEvents.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.threads.events(threadId).path, threadId };
};

const getThreadEventSource = (threadId: string) => {
    return new EventSource(
        ApiRoutes.threads.events(threadId, {
            waitForThread: true,
            follow: true,
            maxRuns: 100,
        }).url
    );
};
getThreadEventSource.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return {
        url: ApiRoutes.threads.events(threadId, {
            waitForThread: true,
            follow: true,
        }).path,
        threadId,
        modifier: "EventSource",
    };
};

const deleteThread = async (threadId: string) => {
    await request({
        url: ApiRoutes.threads.getById(threadId).url,
        method: "DELETE",
        errorMessage: "Failed to delete thread",
    });
};

const getKnowledge = async (threadId: string) => {
    const res = await request<{ items: KnowledgeFile[] }>({
        url: ApiRoutes.threads.getKnowledge(threadId).url,
        errorMessage: "Failed to fetch knowledge for thread",
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
        errorMessage: "Failed to fetch files",
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
    getThreadEventSource,
    deleteThread,
    revalidateThreads,
    getKnowledge,
    getFiles,
};
