import { useLocation, useParams } from "@remix-run/react";
import { MenuIcon } from "lucide-react";
import { $params, $path } from "remix-routes";
import useSWR from "swr";

import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { QueryParamSchemas } from "~/lib/service/routeQueryParams";
import { cn, parseQueryParams } from "~/lib/utils";

import { DarkModeToggle } from "~/components/DarkModeToggle";
import { TypographyH4, TypographySmall } from "~/components/Typography";
import { OttoLogo } from "~/components/branding/OttoLogo";
import { useLayout } from "~/components/layout/LayoutProvider";
import { Button } from "~/components/ui/button";
import { UserMenu } from "~/components/user/UserMenu";

export function HeaderNav() {
    const {
        isExpanded,
        onExpandedChange,
        smallSidebarWidth,
        fullSidebarWidth,
    } = useLayout();

    const { pathname } = useLocation();
    const headerHeight = "h-[60px]";

    return (
        <header
            className={cn(
                "flex transition-all duration-300 ease-in-out",
                headerHeight
            )}
        >
            <div className="h-full flex-auto flex">
                <div
                    className={cn(
                        "relative h-full flex items-center justify-center p-4 border-b",
                        fullSidebarWidth,
                        { "border-r": isExpanded }
                    )}
                >
                    <Button
                        className={cn(
                            "absolute z-30 top-0 left-0 rounded-none",
                            headerHeight,
                            smallSidebarWidth
                        )}
                        variant="ghost"
                        size="icon"
                        onClick={() => onExpandedChange()}
                    >
                        <MenuIcon className="h-6 w-6" />
                        <span className="sr-only">Collapse sidebar</span>
                    </Button>

                    <OttoLogo />
                </div>

                <div className="flex flex-grow border-b">
                    <div className="flex-grow flex justify-start items-center p-4">
                        <TypographyH4 className="text-muted-foreground font-normal w-full">
                            {getHeaderContent(pathname)}
                        </TypographyH4>
                    </div>

                    <div className="flex items-center justify-center p-4 mr-4">
                        <UserMenu className="pr-4 border-r mr-4" />
                        <DarkModeToggle />
                    </div>
                </div>
            </div>
        </header>
    );
}

function getHeaderContent(route: string) {
    if (new RegExp($path("/agents/:agent", { agent: "(.*)" })).test(route)) {
        return <AgentEditContent />;
    }

    if (new RegExp($path("/agents")).test(route)) {
        return <>Agents</>;
    }

    if (new RegExp($path("/threads")).test(route)) {
        return <ThreadsContent />;
    }

    if (new RegExp($path("/thread/:id", { id: "(.*)" })).test(route)) {
        return <ThreadContent />;
    }

    if (new RegExp($path("/users")).test(route)) {
        return <>Users</>;
    }
}

const AgentEditContent = () => {
    const params = useParams();
    const { agent: agentId } = $params("/agents/:agent", params);

    const { data: agent } = useSWR(
        AgentService.getAgentById.key(agentId),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    return <>{agent?.name || "New Agent"}</>;
};

const ThreadsContent = () => {
    const { data: { agentId = null } = {}, success } = parseQueryParams(
        window.location.href,
        QueryParamSchemas.Threads
    );

    const { data: threads } = useSWR(
        ThreadsService.getThreadsByAgent.key(agentId),
        ({ agentId }) => ThreadsService.getThreadsByAgent(agentId)
    );

    const { data: agent } = useSWR(
        AgentService.getAgentById.key(agentId),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    if (!success) return <>Threads</>;

    return (
        <div className="w-full flex justify-between items-center">
            <span>Threads</span>

            {agentId && (
                <TypographySmall className="flex items-center gap-1">
                    <span>
                        Showing <strong>{threads?.length}</strong> threads
                    </span>
                    <span>|</span>
                    <span>
                        Agent: <b>{agent?.name ?? agentId}</b>
                    </span>
                </TypographySmall>
            )}
        </div>
    );
};

const ThreadContent = () => {
    const params = useParams();
    const { id: threadId } = $params("/thread/:id", params);

    const { data: thread } = useSWR(
        ThreadsService.getThreadById.key(threadId),
        ({ threadId }) => ThreadsService.getThreadById(threadId)
    );

    const { data: agent } = useSWR(
        AgentService.getAgentById.key(thread?.agentID),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    return (
        <div className="flex items-center gap-1">
            {agent?.name && (
                <>
                    <span className="text-blue-500">{agent?.name}</span>
                    <span> - </span>
                </>
            )}
            {thread?.description && <span>{thread?.description}</span>}
        </div>
    );
};
