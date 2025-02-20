import { useCallback } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	redirect,
	useLoaderData,
	useMatch,
	useNavigate,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { cn } from "~/lib/utils";

import { Task } from "~/components/task";
import { ScrollArea } from "~/components/ui/scroll-area";

export type SearchParams = RouteQueryParams<"taskSchema">;

export const clientLoader = async ({
	params,
	request,
}: ClientLoaderFunctionArgs) => {
	const { pathParams, query } = RouteService.getRouteInfo(
		"/tasks/:id",
		new URL(request.url),
		params
	);

	if (!pathParams.id) throw redirect($path("/tasks"));

	const [task] = await Promise.all([
		preload(...TaskService.getTaskById.swr({ taskId: pathParams.id })),
		preload(...CronJobApiService.getCronJobs.swr({})),
		preload(WebhookApiService.getWebhooks.key(), () =>
			WebhookApiService.getWebhooks()
		),
	]);

	if (!task) throw redirect($path("/tasks"));

	return { task, threadId: query?.threadId };
};

export default function UserTask() {
	const { task } = useLoaderData<typeof clientLoader>();
	const navigate = useNavigate();
	const onPersistThreadId = useCallback(
		(threadId: string) =>
			navigate($path("/tasks/:id", { id: task.id }, { threadId })),
		[navigate, task.id]
	);

	return (
		<ScrollArea className="h-full" enableScrollStick="bottom">
			<div
				className={cn("relative mx-auto flex h-full max-w-screen-md flex-col")}
			>
				<Task task={task} onPersistThreadId={onPersistThreadId} />
			</div>
		</ScrollArea>
	);
}

const TaskBreadcrumb = () => {
	const match = useMatch("/tasks/:id");

	const { data: task } = useSWR(
		...TaskService.getTaskById.swr({ taskId: match?.params.id })
	);

	return task?.name;
};

export const handle: RouteHandle = {
	breadcrumb: () => [
		{ content: "Tasks", href: $path("/tasks") },
		{ content: <TaskBreadcrumb /> },
	],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
	return [{ title: `Task • ${data?.task.name}` }];
};
