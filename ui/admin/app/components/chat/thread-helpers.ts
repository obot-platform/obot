import { toast } from "sonner";
import useSWR from "swr";

import { UpdateThread } from "~/lib/model/threads";
import { AgentService } from "~/lib/service/api/agentService";
import { KnowledgeFileService } from "~/lib/service/api/knowledgeFileApiService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { useAsync } from "~/hooks/useAsync";

function useThread(threadId?: Nullish<string>) {
    return useSWR(ThreadsService.getThreadById.key(threadId), ({ threadId }) =>
        ThreadsService.getThreadById(threadId)
    );
}

export function useOptimisticThread(threadId?: Nullish<string>) {
    const { data: thread, mutate } = useThread(threadId);

    const handleUpdateThread = useAsync(ThreadsService.updateThreadById);

    const updateThread = async (updates: Partial<UpdateThread>) => {
        if (!thread) return;

        const updatedThread = { ...thread, ...updates };

        // optimistic update
        mutate((thread) => (thread ? updatedThread : thread), false);

        const { error, data } = await handleUpdateThread.executeAsync(
            thread.id,
            updatedThread
        );

        if (data) return;

        if (error instanceof Error) toast.error(error.message);

        // revert optimistic update
        mutate(thread);
    };

    return { thread, updateThread };
}

export function useThreadKnowledge(threadId?: Nullish<string>) {
    return useSWR(
        KnowledgeFileService.getKnowledgeFiles.key("threads", threadId),
        ({ agentId, namespace }) =>
            KnowledgeFileService.getKnowledgeFiles(namespace, agentId)
    );
}

export function useThreadAgents(threadId?: Nullish<string>) {
    const { data: thread } = useThread(threadId);

    return useSWR(
        AgentService.getAgentById.key(thread?.agentID),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );
}
