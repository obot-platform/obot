import { ChatEvent } from "~/lib/model/chatEvents";
import { Thread, UpdateThread } from "~/lib/model/threads";
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

const updateThreadById = async (threadId: string, thread: UpdateThread) => {
    const { data } = await request<Thread>({
        url: ApiRoutes.threads.updateById(threadId).url,
        method: "PUT",
        data: thread,
        errorMessage: "Failed to update thread",
    });

    return data;
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

const abortThread = async (threadId: string) => {
    await request({
        url: ApiRoutes.threads.abortById(threadId).url,
        method: "POST",
        errorMessage: "Failed to abort thread",
    });
};

const revalidateThreads = () =>
    revalidateWhere((url) => url.includes(ApiRoutes.threads.base().path));

export const ThreadsService = {
    getThreads,
    getThreadById,
    getThreadsByAgent,
    getThreadEvents,
    getThreadEventSource,
    updateThreadById,
    deleteThread,
    revalidateThreads,
    getFiles,
    abortThread,
};
