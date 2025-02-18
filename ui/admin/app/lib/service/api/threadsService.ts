import { z } from "zod";

import { ChatEvent } from "~/lib/model/chatEvents";
import { EntityList } from "~/lib/model/primitives";
import { Task } from "~/lib/model/tasks";
import { Thread, UpdateThread } from "~/lib/model/threads";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";
import { QueryService } from "~/lib/service/queryService";
import { downloadUrl } from "~/lib/utils/downloadFile";

const handleGetThreads = createFetcher(
	z.object({}),
	async (_, { signal }) => {
		const { url } = ApiRoutes.threads.base();
		const { data } = await request<EntityList<Thread>>({ url, signal });
		return data.items ?? [];
	},
	() => ApiRoutes.threads.base().path
);

const handleGetById = createFetcher(
	z.object({ id: z.string() }),
	async ({ id }, { signal }) => {
		const { url } = ApiRoutes.threads.getById(id);
		const { data } = await request<Thread>({ url, signal });
		return data;
	},
	() => ApiRoutes.threads.getById(":threadId").path
);

const updateThreadById = async (threadId: string, thread: UpdateThread) => {
	const { data } = await request<Thread>({
		url: ApiRoutes.threads.updateById(threadId).url,
		method: "PUT",
		data: thread,
		errorMessage: "Failed to update thread",
	});

	return data;
};

const handleGetByAgent = createFetcher(
	z.object({ agentId: z.string() }),
	async ({ agentId }, { signal }) => {
		const { data } = await request<EntityList<Thread>>({
			url: ApiRoutes.threads.getByAgent(agentId).url,
			signal,
			errorMessage: "Failed to fetch threads by agent id",
		});
		return data.items ?? [];
	},
	() => ApiRoutes.threads.getByAgent(":agentId").path
);
const handleGetByAgent1 = async (agentId: string) => {
	const res = await request<{ items: Thread[] }>({
		url: ApiRoutes.threads.getByAgent(agentId).url,
		errorMessage: "Failed to fetch threads by agent",
	});

	return res.data.items ?? ([] as Thread[]);
};
handleGetByAgent1.key = (agentId?: Nullish<string>) => {
	if (!agentId) return null;

	return { url: ApiRoutes.threads.getByAgent(agentId).path, agentId };
};

const handleGetThreadEvents = createFetcher(
	z.object({ threadId: z.string() }),
	async ({ threadId }, { signal }) => {
		const { data } = await request<EntityList<ChatEvent>>({
			url: ApiRoutes.threads.events(threadId).url,
			headers: { Accept: "application/json" },
			errorMessage: "Failed to fetch thread events",
			signal,
		});
		return data.items ?? [];
	},
	() => ApiRoutes.threads.events(":threadId").path
);

const getThreadEventSource = (threadId: string) => {
	return new EventSource(
		ApiRoutes.threads.events(threadId, {
			waitForThread: true,
			follow: true,
			maxRuns: 100,
		}).url
	);
};

const handleGetTasks = createFetcher(
	z.object({ threadId: z.string() }),
	async ({ threadId }, { signal }) => {
		const { url } = ApiRoutes.threads.getTasksForThread(threadId);
		const { data } = await request<EntityList<Task>>({ url, signal });

		return data.items ?? [];
	},
	() => ApiRoutes.threads.getTasksForThread(":threadId").path
);

const deleteThread = async (threadId: string) => {
	await request({
		url: ApiRoutes.threads.getById(threadId).url,
		method: "DELETE",
		errorMessage: "Failed to delete thread",
	});
};

const handleGetFiles = createFetcher(
	QueryService.queryable.extend({
		threadId: z.string(),
		filters: z.object({ search: z.string().optional() }).optional(),
	}),
	async ({ threadId, query, filters }, { signal }) => {
		const { data } = await request<EntityList<WorkspaceFile>>({
			url: ApiRoutes.threads.getFiles(threadId).url,
			errorMessage: "Failed to fetch files for thread",
			signal,
		});

		const { search } = filters ?? {};

		if (search)
			data.items = data.items?.filter((i) =>
				i.name.toLowerCase().includes(search?.toLowerCase())
			);

		return QueryService.paginate(data.items ?? [], query.pagination);
	},
	() => ApiRoutes.threads.getFiles(":threadId").path
);

const downloadFile = (threadId: string, filePath: string) => {
	downloadUrl(ApiRoutes.threads.downloadFile(threadId, filePath).url, filePath);
};

const abortThread = async (threadId: string) => {
	await request({
		url: ApiRoutes.threads.abortById(threadId).url,
		method: "POST",
		errorMessage: "Failed to abort thread",
	});
};

export const ThreadsService = {
	getThreads: handleGetThreads,
	getThreadById: handleGetById,
	getThreadsByAgent: handleGetByAgent,
	getThreadEvents: handleGetThreadEvents,
	getTasksForThread: handleGetTasks,
	getFiles: handleGetFiles,
	getThreadEventSource,
	updateThreadById,
	deleteThread,
	downloadFile,
	abortThread,
};
