import { z } from "zod";

import { Task, UpdateTask } from "~/lib/model/tasks";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { ResponseHeaders, request } from "~/lib/service/api/primitives";
import {
	createFetcher,
	createMutator,
} from "~/lib/service/api/service-primitives";

const getTasks = createFetcher(
	z.object({}),
	async (_, { signal }) => {
		const res = await request<{ items: Task[] }>({
			url: ApiRoutes.tasks.base().url,
			errorMessage: "Failed to fetch tasks",
			signal,
		});

		return res.data.items ?? ([] as Task[]);
	},
	() => ApiRoutes.tasks.base().path
);

const getTaskById = createFetcher(
	z.object({ taskId: z.string() }),
	async ({ taskId }, { signal }) => {
		const res = await request<Task>({
			url: ApiRoutes.tasks.getById(taskId).url,
			errorMessage: "Failed to fetch task",
			signal,
		});

		return res.data;
	},
	() => ApiRoutes.tasks.getById(":taskId").path
);

const updateTask = createMutator(
	async ({ id, task }: { id: string; task: UpdateTask }, { signal }) => {
		const res = await request<Task>({
			url: ApiRoutes.tasks.getById(id).url,
			method: "PUT",
			data: task,
			errorMessage: "Failed to update task",
			signal,
		});

		return res.data;
	}
);

const revalidateTasks = () =>
	revalidateWhere((url) => url.includes(ApiRoutes.tasks.base().path));

async function authenticateTask(taskId: string) {
	const response = await request<ReadableStream>({
		url: ApiRoutes.tasks.authenticate(taskId).url,
		method: "POST",
		headers: { Accept: "text/event-stream" },
		responseType: "stream",
		errorMessage: "Failed to invoke authenticate task",
	});

	const reader = response.data
		?.pipeThrough(new TextDecoderStream())
		.getReader();

	const threadId = response.headers[ResponseHeaders.ThreadId] as string;

	return { reader, threadId };
}

export const TaskService = {
	getTasks,
	getTaskById,
	updateTask,
	revalidateTasks,
	authenticateTask,
};
