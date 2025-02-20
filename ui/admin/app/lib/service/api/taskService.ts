import { Task, UpdateTask } from "~/lib/model/tasks";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { ResponseHeaders, request } from "~/lib/service/api/primitives";

async function getTasks() {
	const res = await request<{ items: Task[] }>({
		url: ApiRoutes.tasks.base().url,
		errorMessage: "Failed to fetch tasks",
	});

	return res.data.items ?? ([] as Task[]);
}
getTasks.key = () => ({ url: ApiRoutes.tasks.base().path }) as const;

const getTaskById = async (taskId: string) => {
	const res = await request<Task>({
		url: ApiRoutes.tasks.getById(taskId).url,
		errorMessage: "Failed to fetch task",
	});

	return res.data;
};
getTaskById.key = (taskId?: Nullish<string>) => {
	if (!taskId) return null;

	return { url: ApiRoutes.tasks.getById(taskId).path, taskId };
};

async function updateTask({ id, task }: { id: string; task: UpdateTask }) {
	const res = await request<Task>({
		url: ApiRoutes.tasks.getById(id).url,
		method: "PUT",
		data: task,
		errorMessage: "Failed to update task",
	});

	return res.data;
}

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
