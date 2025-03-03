import { ArrowLeftIcon } from "lucide-react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	redirect,
	useLoaderData,
	useMatch,
	useNavigate,
} from "react-router";
import { $path } from "safe-routes";
import { preload } from "swr";

import { KnowledgeFileNamespace } from "~/lib/model/knowledge";
import { KnowledgeFileService } from "~/lib/service/api/knowledgeFileApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";

import { Chat } from "~/components/chat";
import { ChatProvider } from "~/components/chat/ChatContext";
import { ThreadMeta } from "~/components/thread/ThreadMeta";
import { Button } from "~/components/ui/button";
import {
	ResizableHandle,
	ResizablePanel,
	ResizablePanelGroup,
} from "~/components/ui/resizable";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export const clientLoader = async ({
	params,
	request,
}: ClientLoaderFunctionArgs) => {
	const routeInfo = RouteService.getRouteInfo(
		"/task-runs/:id",
		new URL(request.url),
		params
	);

	const { id } = routeInfo.pathParams;

	if (!id) {
		throw redirect("/threads");
	}

	const thread = await preload(...ThreadsService.getThreadById.swr({ id }));
	if (!thread) throw redirect("/threads");

	const [task] = await Promise.all([
		thread.workflowID
			? preload(...TaskService.getTaskById.swr({ taskId: thread.workflowID }))
			: null,
		preload(
			KnowledgeFileService.getKnowledgeFiles.key(
				KnowledgeFileNamespace.Threads,
				thread.id
			),
			() =>
				KnowledgeFileService.getKnowledgeFiles(
					KnowledgeFileNamespace.Threads,
					thread.id
				)
		),
	]);

	if (!task) throw redirect("/tasks");

	return { thread, task };
};

export default function TaskRuns() {
	const { thread, task } = useLoaderData<typeof clientLoader>();

	const navigate = useNavigate();
	return (
		<div className="relative flex h-full flex-col overflow-hidden">
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="outline"
						onClick={() => navigate(-1)}
						className="ml-4"
					>
						<ArrowLeftIcon className="h-4 w-4" />
					</Button>
				</TooltipTrigger>
				<TooltipContent>Go Back</TooltipContent>
			</Tooltip>

			<ResizablePanelGroup direction="horizontal" className="flex-auto">
				<ResizablePanel defaultSize={70} minSize={25}>
					<ChatProvider id={task.id} mode="agent" readOnly threadId={thread.id}>
						<Chat />
					</ChatProvider>
				</ResizablePanel>
				<ResizableHandle />
				<ResizablePanel defaultSize={30} minSize={25}>
					<ScrollArea className="h-full">
						<ThreadMeta
							className="rounded-none border-none"
							thread={thread}
							entity={task}
						/>
					</ScrollArea>
				</ResizablePanel>
			</ResizablePanelGroup>
		</div>
	);
}

const ThreadBreadcrumb = () => useMatch("/task-runs/:id")?.params.id;

export const handle: RouteHandle = {
	breadcrumb: () => [
		{ content: "Task Runs", href: $path("/task-runs") },
		{ content: <ThreadBreadcrumb /> },
	],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
	return [{ title: `Task Run • ${data?.thread.id}` }];
};
