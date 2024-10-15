import {
    ClientLoaderFunctionArgs,
    Link,
    redirect,
    useLoaderData,
} from "@remix-run/react";
import { ArrowLeftIcon } from "lucide-react";
import { $params } from "remix-routes";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";

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
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export const clientLoader = async ({ params }: ClientLoaderFunctionArgs) => {
    const { id } = $params("/thread/:id", params);

    const thread = await ThreadsService.getThreadById(id);
    if (!thread) throw redirect("/threads");

    const agent = thread.agentID
        ? await AgentService.getAgentById(thread.agentID)
        : null;
    const files = await ThreadsService.getFiles(id);
    const knowledge = await ThreadsService.getKnowledge(id);

    return { thread, agent, files, knowledge };
};

export default function ChatAgent() {
    const { thread, agent, files, knowledge } =
        useLoaderData<typeof clientLoader>();

    return (
        <ChatProvider
            id={agent?.id || ""}
            mode="agent"
            threadId={thread.id}
            readOnly
        >
            <div className="h-full flex flex-col overflow-hidden relative">
                <TooltipProvider>
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
                </TooltipProvider>
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
                                agent={agent ?? ({} as Agent)}
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
