import { ArrowLeftIcon } from "lucide-react";
import {
    ClientLoaderFunctionArgs,
    Link,
    redirect,
    useLoaderData,
    useMatch,
} from "react-router";
import { $path } from "safe-routes";

import { KnowledgeFileNamespace } from "~/lib/model/knowledge";
import { AgentService } from "~/lib/service/api/agentService";
import { KnowledgeFileService } from "~/lib/service/api/knowledgeFileApiService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";
import { noop } from "~/lib/utils";

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
        "/threads/:id",
        new URL(request.url),
        params
    );

    const { id } = routeInfo.pathParams;

    if (!id) {
        throw redirect("/threads");
    }

    const thread = await ThreadsService.getThreadById(id);
    if (!thread) throw redirect("/threads");

    const agent = thread.agentID
        ? await AgentService.getAgentById(thread.agentID).catch(noop)
        : null;

    const workflow = thread.workflowID
        ? await WorkflowService.getWorkflowById(thread.workflowID).catch(noop)
        : null;

    const files = await ThreadsService.getFiles(id);
    const knowledge = await KnowledgeFileService.getKnowledgeFiles(
        KnowledgeFileNamespace.Threads,
        thread.id
    );

    return { thread, agent, workflow, files, knowledge };
};

export default function ChatAgent() {
    const { thread, agent, workflow, files, knowledge } =
        useLoaderData<typeof clientLoader>();

    const getEntity = () => {
        if (agent) return agent;
        if (workflow) return workflow;
        throw new Error("Trying to view a thread with an unsupported parent.");
    };

    const [isAgent, entity] = [agent !== null, getEntity()];

    return (
        <div className="h-full flex flex-col overflow-hidden relative">
            <Tooltip>
                <Button
                    variant="outline"
                    size="icon"
                    className="absolute top-4 left-4 z-10"
                    asChild
                >
                    <TooltipTrigger>
                        <Link to="/threads">
                            <ArrowLeftIcon className="h-4 w-4" />
                        </Link>
                    </TooltipTrigger>
                </Button>
                <TooltipContent>Go Back</TooltipContent>
            </Tooltip>

            <ResizablePanelGroup direction="horizontal" className="flex-auto">
                <ResizablePanel defaultSize={70} minSize={25}>
                    <ChatProvider
                        id={entity.id}
                        mode="agent"
                        threadId={thread.id}
                        readOnly={!isAgent}
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
                            for={entity}
                            files={files}
                            knowledge={knowledge}
                        />
                    </ScrollArea>
                </ResizablePanel>
            </ResizablePanelGroup>
        </div>
    );
}

const ThreadBreadcrumb = () => useMatch("/threads/:id")?.params.id;

export const handle: RouteHandle = {
    breadcrumb: () => [
        { content: "Threads", href: $path("/threads") },
        { content: <ThreadBreadcrumb /> },
    ],
};
