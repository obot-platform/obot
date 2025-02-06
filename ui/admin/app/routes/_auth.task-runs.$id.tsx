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
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
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

	const thread = await preload(ThreadsService.getThreadById.key(id), () =>
		ThreadsService.getThreadById(id)
	);
	if (!thread) throw redirect("/threads");

	const [workflow] = await Promise.all([
		thread.workflowID
			? preload(WorkflowService.getWorkflowById.key(thread.workflowID), () =>
					WorkflowService.getWorkflowById(thread.workflowID)
				)
			: null,
		preload(ThreadsService.getFiles.key(thread.id), () =>
			ThreadsService.getFiles(thread.id)
		),
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

	if (!workflow) throw redirect("/tasks");

	return { thread, workflow };
};

export default function TaskRuns() {
	const { thread, workflow } = useLoaderData<typeof clientLoader>();

	const navigate = useNavigate();
	return (
		<div className="relative flex h-full flex-col overflow-hidden">
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						variant="outline"
						size="icon"
						className="absolute left-4 top-4 z-10"
						onClick={() => navigate(-1)}
					>
						<ArrowLeftIcon className="h-4 w-4" />
					</Button>
				</TooltipTrigger>
				<TooltipContent>Go Back</TooltipContent>
			</Tooltip>

			<ResizablePanelGroup direction="horizontal" className="flex-auto">
				<ResizablePanel defaultSize={70} minSize={25}>
					<ChatProvider
						id={workflow.id}
						mode="agent"
						readOnly
						threadId={thread.id}
					>
						<Chat />
					</ChatProvider>
				</ResizablePanel>
				<ResizableHandle />
				<ResizablePanel defaultSize={30} minSize={25}>
					<ScrollArea className="h-full">
						<ThreadMeta
							className="rounded-none border-none"
							thread={thread}
							entity={workflow}
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
