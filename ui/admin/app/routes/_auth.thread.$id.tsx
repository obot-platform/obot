import {
    ClientLoaderFunctionArgs,
    Link,
    redirect,
    useLoaderData,
} from "@remix-run/react";
import { ArrowLeftIcon } from "lucide-react";

import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
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
        "/thread/:id",
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
    const knowledge = await ThreadsService.getKnowledge(id);

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

    return (
        <ChatProvider
            id={agent?.id || ""}
            mode="agent"
            threadId={thread.id}
            readOnly
        >
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

                <ResizablePanelGroup
                    direction="horizontal"
                    className="flex-auto"
                >
                    <ResizablePanel defaultSize={70} minSize={25}>
                        <Chat />
                    </ResizablePanel>
                    <ResizableHandle />
                    <ResizablePanel defaultSize={30} minSize={25}>
                        <ScrollArea className="h-full">
                            <ThreadMeta
                                className="rounded-none border-none"
                                thread={thread}
                                for={getEntity()}
                                files={files}
                                knowledge={knowledge}
                            />
                        </ScrollArea>
                    </ResizablePanel>
                </ResizablePanelGroup>
            </div>
        </ChatProvider>
    );
}
