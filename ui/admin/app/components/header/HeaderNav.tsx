import { Link } from "@remix-run/react";
import { $path } from "remix-routes";
import useSWR from "swr";

import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { cn } from "~/lib/utils";

import { DarkModeToggle } from "~/components/DarkModeToggle";
import {
    Breadcrumb,
    BreadcrumbItem,
    BreadcrumbLink,
    BreadcrumbList,
    BreadcrumbPage,
    BreadcrumbSeparator,
} from "~/components/ui/breadcrumb";
import { SidebarTrigger } from "~/components/ui/sidebar";
import { UserMenu } from "~/components/user/UserMenu";
import { useUnknownPathParams } from "~/hooks/useRouteInfo";

export function HeaderNav() {
    const headerHeight = "h-[60px]";

    return (
        <header
            className={cn(
                "flex transition-all duration-300 ease-in-out",
                headerHeight
            )}
        >
            <div className="h-full flex-auto flex">
                <div className="flex flex-grow z-20">
                    <div className="flex-grow flex justify-start items-center p-4">
                        <SidebarTrigger className="h-4 w-4" />
                        <div className="border-r h-4 mx-4" />
                        <RouteBreadcrumbs />
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

function RouteBreadcrumbs() {
    const routeInfo = useUnknownPathParams();

    return (
        <Breadcrumb>
            <BreadcrumbList>
                <BreadcrumbItem>
                    <BreadcrumbLink asChild>
                        <Link to={$path("/")}>Home</Link>
                    </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator />

                {routeInfo?.path === "/agents/:agent" && (
                    <>
                        <BreadcrumbItem>
                            <BreadcrumbLink asChild>
                                <Link to={$path("/agents")}>Agents</Link>
                            </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                            <BreadcrumbPage>
                                <AgentName
                                    agentId={routeInfo.pathParams.agent || ""}
                                />
                            </BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/agents" && (
                    <BreadcrumbItem>
                        <BreadcrumbPage>Agents</BreadcrumbPage>
                    </BreadcrumbItem>
                )}

                {routeInfo?.path === "/threads" && (
                    <>
                        {routeInfo.query?.from && (
                            <>
                                <BreadcrumbItem>
                                    <BreadcrumbLink asChild>
                                        {renderThreadFrom(routeInfo.query.from)}
                                    </BreadcrumbLink>
                                </BreadcrumbItem>

                                <BreadcrumbSeparator />
                            </>
                        )}

                        <BreadcrumbItem>
                            <BreadcrumbPage>Threads</BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/thread/:id" && (
                    <>
                        <BreadcrumbItem>
                            <BreadcrumbLink asChild>
                                <Link to={$path("/threads")}>Threads</Link>
                            </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                            <BreadcrumbPage>
                                <ThreadName
                                    threadId={routeInfo.pathParams.id || ""}
                                />
                            </BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/workflows/:workflow" && (
                    <>
                        <BreadcrumbItem>
                            <BreadcrumbLink asChild>
                                <Link to={$path("/workflows")}>Workflows</Link>
                            </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                            <BreadcrumbPage>
                                <WorkflowName
                                    workflowId={
                                        routeInfo.pathParams.workflow || ""
                                    }
                                />
                            </BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/webhooks" && (
                    <BreadcrumbItem>
                        <BreadcrumbPage>Webhooks</BreadcrumbPage>
                    </BreadcrumbItem>
                )}

                {routeInfo?.path === "/webhooks/create" && (
                    <>
                        <BreadcrumbItem>
                            <BreadcrumbLink asChild>
                                <Link to={$path("/webhooks")}>Webhooks</Link>
                            </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                            <BreadcrumbPage>Create Webhook</BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/webhooks/:webhook" && (
                    <>
                        <BreadcrumbItem>
                            <BreadcrumbLink asChild>
                                <Link to={$path("/webhooks")}>Webhooks</Link>
                            </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                            <BreadcrumbPage>
                                <WebhookName
                                    webhookId={
                                        routeInfo.pathParams.webhook || ""
                                    }
                                />
                            </BreadcrumbPage>
                        </BreadcrumbItem>
                    </>
                )}

                {routeInfo?.path === "/tools" && (
                    <BreadcrumbItem>
                        <BreadcrumbPage>Tools</BreadcrumbPage>
                    </BreadcrumbItem>
                )}
                {routeInfo?.path === "/users" && (
                    <BreadcrumbItem>
                        <BreadcrumbPage>Users</BreadcrumbPage>
                    </BreadcrumbItem>
                )}
                {routeInfo?.path === "/oauth-apps" && (
                    <BreadcrumbItem>
                        <BreadcrumbPage>OAuth Apps</BreadcrumbPage>
                    </BreadcrumbItem>
                )}
            </BreadcrumbList>
        </Breadcrumb>
    );
}

const renderThreadFrom = (from: "agents" | "workflows" | "users") => {
    if (from === "agents") return <Link to={$path("/agents")}>Agents</Link>;

    if (from === "workflows")
        return <Link to={$path("/workflows")}>Workflows</Link>;

    if (from === "users") return <Link to={$path("/users")}>Users</Link>;
};

const AgentName = ({ agentId }: { agentId: string }) => {
    const { data: agent } = useSWR(
        AgentService.getAgentById.key(agentId),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    return <>{agent?.name || "New Agent"}</>;
};

const WorkflowName = ({ workflowId }: { workflowId: string }) => {
    const { data: workflow } = useSWR(
        WorkflowService.getWorkflowById.key(workflowId),
        ({ workflowId }) => WorkflowService.getWorkflowById(workflowId)
    );

    return <>{workflow?.name || "New Workflow"}</>;
};

const ThreadName = ({ threadId }: { threadId: string }) => {
    const { data: thread } = useSWR(
        ThreadsService.getThreadById.key(threadId),
        ({ threadId }) => ThreadsService.getThreadById(threadId)
    );

    return <>{thread?.description || threadId}</>;
};

const WebhookName = ({ webhookId }: { webhookId: string }) => {
    const { data } = useSWR(
        WebhookApiService.getWebhookById.key(webhookId),
        ({ id }) => WebhookApiService.getWebhookById(id)
    );

    return <>{data?.name || webhookId}</>;
};
