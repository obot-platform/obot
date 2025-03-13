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
import { AgentService } from "~/lib/service/api/agentService";
import { KnowledgeFileService } from "~/lib/service/api/knowledgeFileApiService";
import { ProjectApiService } from "~/lib/service/api/projectApiService";
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
		"/chat-threads/:id",
		new URL(request.url),
		params
	);

	const { id } = routeInfo.pathParams;

	if (!id) {
		throw redirect("/threads");
	}

	const thread = await preload(...ThreadsService.getThreadById.swr({ id }));
	if (!thread) throw redirect("/threads");

	const [agent, project] = await Promise.all([
		preload(...AgentService.getAgentById.swr({ agentId: thread.assistantID })),
		preload(...ProjectApiService.getById.swr({ id: thread.projectID })),
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

	return { thread, agent, project };
};

export default function ChatThread() {
	const { thread, agent, project } = useLoaderData<typeof clientLoader>();

	const navigate = useNavigate();
	return (
		<div className="relative flex h-full flex-col overflow-hidden">
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						variant="outline"
						size="icon"
						className="absolute left-4 top-4 z-10"
					>
						<Button size="icon" variant="outline" onClick={() => navigate(-1)}>
							<ArrowLeftIcon className="h-4 w-4" />
						</Button>
					</Button>
				</TooltipTrigger>
				<TooltipContent>Go Back</TooltipContent>
			</Tooltip>

			<ResizablePanelGroup direction="horizontal" className="flex-auto">
				<ResizablePanel defaultSize={70} minSize={25}>
					<ChatProvider
						id={agent.id}
						mode="agent"
						threadId={thread.id}
						introductionMessage={agent.introductionMessage}
						starterMessages={agent.starterMessages}
						icons={agent.icons}
						name={agent.name}
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
							entity={agent}
							project={project}
						/>
					</ScrollArea>
				</ResizablePanel>
			</ResizablePanelGroup>
		</div>
	);
}

const ThreadBreadcrumb = () => useMatch("/chat-threads/:id")?.params.id;

export const handle: RouteHandle = {
	breadcrumb: () => [
		{ content: "Chat Threads", href: $path("/chat-threads") },
		{ content: <ThreadBreadcrumb /> },
	],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
	return [{ title: `Chat Thread • ${data?.thread.id}` }];
};
