import { toast } from "sonner";
import useSWR from "swr";

import { CredentialNamespace } from "~/lib/model/credentials";
import { KnowledgeFileNamespace } from "~/lib/model/knowledge";
import { UpdateThread } from "~/lib/model/threads";
import { AgentService } from "~/lib/service/api/agentService";
import { CredentialApiService } from "~/lib/service/api/credentialApiService";
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

		const [error, data] = await handleUpdateThread.executeAsync(
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
		KnowledgeFileService.getKnowledgeFiles.key(
			KnowledgeFileNamespace.Threads,
			threadId
		),
		({ entityId, namespace }) =>
			KnowledgeFileService.getKnowledgeFiles(namespace, entityId)
	);
}

export function useThreadFiles(threadId?: Nullish<string>) {
	return useSWR(ThreadsService.getFiles.key(threadId), ({ threadId }) =>
		ThreadsService.getFiles(threadId)
	);
}

export function useThreadAgents(threadId?: Nullish<string>) {
	const { data: thread } = useThread(threadId);

	return useSWR(AgentService.getAgentById.key(thread?.agentID), ({ agentId }) =>
		AgentService.getAgentById(agentId)
	);
}

export function useThreadCredentials(threadId: Nullish<string>) {
	const getCredentials = useSWR(
		CredentialApiService.getCredentials.key(
			CredentialNamespace.Threads,
			threadId
		),
		({ namespace, entityId }) =>
			CredentialApiService.getCredentials(namespace, entityId)
	);

	const handleDeleteCredential = async (credentialName: string) => {
		if (!threadId) return;

		return await CredentialApiService.deleteCredential(
			CredentialNamespace.Threads,
			threadId,
			credentialName
		);
	};

	const deleteCredential = useAsync(handleDeleteCredential, {
		onSuccess: (credentialId) => {
			getCredentials.mutate((creds) =>
				creds?.filter((c) => c.name !== credentialId)
			);
		},
	});

	return { getCredentials, deleteCredential };
}
