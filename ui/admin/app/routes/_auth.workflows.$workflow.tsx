import { useCallback } from "react";
import {
    ClientLoaderFunctionArgs,
    redirect,
    useLoaderData,
    useMatch,
    useNavigate,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";

import { Chat } from "~/components/chat";
import { ChatProvider } from "~/components/chat/ChatContext";
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "~/components/ui/resizable";
import { Workflow } from "~/components/workflow";

export type SearchParams = RouteQueryParams<"workflowSchema">;

export const clientLoader = async ({
    params,
    request,
}: ClientLoaderFunctionArgs) => {
    const { pathParams, query } = RouteService.getRouteInfo(
        "/workflows/:workflow",
        new URL(request.url),
        params
    );

    if (!pathParams.workflow) throw redirect($path("/workflows"));

    const workflow = await preload(
        WorkflowService.getWorkflowById.key(pathParams.workflow),
        () => WorkflowService.getWorkflowById(pathParams.workflow)
    );

    if (!workflow) throw redirect($path("/workflows"));

    return { workflow, threadId: query?.threadId };
};

export default function ChatAgent() {
    const { workflow, threadId } = useLoaderData<typeof clientLoader>();

    const navigate = useNavigate();

    const onPersistThreadId = useCallback(
        (threadId: string) =>
            navigate(
                $path(
                    "/workflows/:workflow",
                    { workflow: workflow.id },
                    { threadId }
                )
            ),
        [navigate, workflow.id]
    );

    return (
        <div className="h-full flex flex-col overflow-hidden relative">
            <ChatProvider
                id={workflow.id}
                mode="workflow"
                threadId={threadId}
                onCreateThreadId={onPersistThreadId}
            >
                <ResizablePanelGroup
                    direction="horizontal"
                    className="flex-auto"
                >
                    <ResizablePanel className="">
                        <Workflow
                            workflow={workflow}
                            onPersistThreadId={onPersistThreadId}
                        />
                    </ResizablePanel>
                    <ResizableHandle withHandle />
                    <ResizablePanel>
                        <Chat className="bg-background-secondary" />
                    </ResizablePanel>
                </ResizablePanelGroup>
            </ChatProvider>
        </div>
    );
}

const WorkflowBreadcrumb = () => {
    const match = useMatch("/workflows/:workflow");

    const { data: workflow } = useSWR(
        WorkflowService.getWorkflowById.key(match?.params.workflow || ""),
        ({ workflowId }) => WorkflowService.getWorkflowById(workflowId)
    );

    return workflow?.name;
};

export const handle: RouteHandle = {
    breadcrumb: () => [{ content: <WorkflowBreadcrumb /> }],
};
