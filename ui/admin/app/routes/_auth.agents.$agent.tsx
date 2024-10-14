import { ArrowLeftIcon } from "@radix-ui/react-icons";
import {
    ClientLoaderFunctionArgs,
    Link,
    redirect,
    useLoaderData,
    useNavigate,
} from "@remix-run/react";
import { useCallback } from "react";
import { $params, $path } from "remix-routes";
import { z } from "zod";

import { AgentService } from "~/lib/service/api/agentService";
import { noop, parseQueryParams } from "~/lib/utils";

import { Agent } from "~/components/agent";
import { Chat, ChatProvider } from "~/components/chat";
import { Button } from "~/components/ui/button";
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "~/components/ui/resizable";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

const paramSchema = z.object({
    threadId: z.string().optional(),
    from: z.string().optional(),
});

export type SearchParams = z.infer<typeof paramSchema>;

export const clientLoader = async ({
    params,
    request,
}: ClientLoaderFunctionArgs) => {
    const { agent: agentId } = $params("/agents/:agent", params);
    const { threadId, from } =
        parseQueryParams(request.url, paramSchema).data || {};

    // preload the agent
    const agent = await AgentService.getAgentById(agentId).catch(noop);

    if (!agent) {
        throw redirect("/agents");
    }
    return { agent, threadId, from };
};

export default function ChatAgent() {
    const { agent, threadId, from } = useLoaderData<typeof clientLoader>();
    const navigate = useNavigate();

    const updateThreadId = useCallback(
        (newThreadId?: Nullish<string>) => {
            navigate(
                $path(
                    "/agents/:agent",
                    { agent: agent.id },
                    newThreadId ? { threadId: newThreadId } : undefined
                )
            );
        },
        [agent, navigate]
    );

    return (
        <div className="h-full flex flex-col overflow-hidden relative">
            <ChatProvider
                id={agent.id}
                threadId={threadId}
                onCreateThreadId={updateThreadId}
            >
                <ResizablePanelGroup
                    direction="horizontal"
                    className="flex-auto"
                >
                    <ResizablePanel>
                        <TooltipProvider>
                            <Tooltip>
                                <Button
                                    variant="outline"
                                    size="icon"
                                    className="m-4"
                                    asChild
                                >
                                    <TooltipTrigger>
                                        <Link to={from ?? "/agents"}>
                                            <ArrowLeftIcon className="h-4 w-4" />
                                        </Link>
                                    </TooltipTrigger>
                                </Button>
                                <TooltipContent>Go Back</TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                        <Agent
                            agent={agent}
                            onRefresh={() => updateThreadId(null)}
                        />
                    </ResizablePanel>
                    <ResizableHandle withHandle />
                    <ResizablePanel>
                        <Chat />
                    </ResizablePanel>
                </ResizablePanelGroup>
            </ChatProvider>
        </div>
    );
}
