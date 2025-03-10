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

import { AgentService } from "~/lib/service/api/agentService";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { ProjectApiService } from "~/lib/service/api/projectApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { cn } from "~/lib/utils";

import { Task } from "~/components/task";
import { TaskMeta } from "~/components/task/TaskMeta";
import {
	ResizableHandle,
	ResizablePanel,
	ResizablePanelGroup,
} from "~/components/ui/resizable";
import { ScrollArea } from "~/components/ui/scroll-area";

export type SearchParams = RouteQueryParams<"taskSchema">;

export const clientLoader = async ({
	params,
	request,
}: ClientLoaderFunctionArgs) => {
	const { pathParams } = RouteService.getRouteInfo(
		"/tasks/:id",
		new URL(request.url),
		params
	);

	if (!pathParams.id) throw redirect($path("/tasks"));

	const [task, threads] = await Promise.all([
		preload(...TaskService.getTaskById.swr({ taskId: pathParams.id })),
		preload(...ThreadsService.getThreads.swr({})),
		preload(...CronJobApiService.getCronJobs.swr({})),
		preload(WebhookApiService.getWebhooks.key(), () =>
			WebhookApiService.getWebhooks()
		),
	]);

	if (!task) throw redirect($path("/tasks"));

	const project = await preload(
		...ProjectApiService.getById.swr({ id: task.projectID })
	);
	const agent = await preload(
		...AgentService.getAgentById.swr({ agentId: project.assistantID })
	);
	const taskRuns = threads.filter((t) => t.workflowID === task.id).length;

	return { task, agent, taskRuns, project };
};

export default function UserTask() {
	const pageData = useLoaderData<typeof clientLoader>();
	const { task } = pageData;

	const navigate = useNavigate();
	const onPersistThreadId = useCallback(
		(threadId: string) =>
			navigate($path("/tasks/:id", { id: task.id }, { threadId })),
		[navigate, task.id]
	);

	return (
		<ResizablePanelGroup direction="horizontal" className="flex-auto">
			<ResizablePanel defaultSize={70} minSize={25}>
				<ScrollArea className="h-full" enableScrollStick="bottom">
					<div className={cn("relative mx-auto flex h-full flex-col")}>
						<Task task={task} onPersistThreadId={onPersistThreadId} />
					</div>
				</ScrollArea>
			</ResizablePanel>
			<ResizableHandle />
			<ResizablePanel defaultSize={30} minSize={25}>
				<ScrollArea className="h-full">
					<TaskMeta {...pageData} />
				</ScrollArea>
			</ResizablePanel>
		</ResizablePanelGroup>
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
