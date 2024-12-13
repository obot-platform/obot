import { useCallback } from "react";
import {
    ClientLoaderFunctionArgs,
    redirect,
    useLoaderData,
    useMatch,
    useNavigate,
} from "react-router";
import { $path } from "remix-routes";
import useSWR, { preload } from "swr";

import { AgentService } from "~/lib/service/api/agentService";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { noop } from "~/lib/utils";

import { Agent } from "~/components/agent";
import { AgentProvider } from "~/components/agent/AgentContext";
import { Chat, ChatProvider } from "~/components/chat";
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "~/components/ui/resizable";

export type SearchParams = RouteQueryParams<"agentSchema">;

export const clientLoader = async ({
    params,
    request,
}: ClientLoaderFunctionArgs) => {
    const url = new URL(request.url);

    const routeInfo = RouteService.getRouteInfo("/agents/:agent", url, params);

    const { agent: agentId } = routeInfo.pathParams;
    const { threadId, from } = routeInfo.query ?? {};

    if (!agentId) {
        throw redirect("/agents");
    }

    await preload(
        DefaultModelAliasApiService.getAliases.key(),
        DefaultModelAliasApiService.getAliases
    );

    // preload the agent
    const agent = await AgentService.getAgentById(agentId).catch(noop);

    if (!agent) {
        throw redirect("/agents");
    }
    return { agent, threadId, from };
};

export default function ChatAgent() {
    const { agent, threadId } = useLoaderData<typeof clientLoader>();
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
            <ResizablePanelGroup direction="horizontal" className="flex-auto">
                <ResizablePanel className="">
                    <AgentProvider agent={agent}>
                        <Agent onRefresh={updateThreadId} key={agent.id} />
                    </AgentProvider>
                </ResizablePanel>
                <ResizableHandle withHandle />
                <ResizablePanel>
                    <ChatProvider
                        id={agent.id}
                        threadId={threadId}
                        onCreateThreadId={updateThreadId}
                    >
                        <Chat className="bg-sidebar" />
                    </ChatProvider>
                </ResizablePanel>
            </ResizablePanelGroup>
        </div>
    );
}

const AgentBreadcrumb = () => {
    const match = useMatch("/agents/:agent");

    const { data: agent } = useSWR(
        AgentService.getAgentById.key(match?.params.agent),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    return <>{agent?.name || "New Agent"}</>;
};

export const handle: RouteHandle = {
    breadcrumb: () => [{ content: <AgentBreadcrumb /> }],
};
