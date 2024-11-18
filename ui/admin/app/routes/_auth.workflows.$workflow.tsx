import {
    ClientLoaderFunctionArgs,
    redirect,
    useLoaderData,
    useNavigate,
} from "@remix-run/react";
import { $path } from "remix-routes";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { noop } from "~/lib/utils";

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

    if (!pathParams.workflow) {
        throw redirect($path("/workflows"));
    }

    const workflow = await WorkflowService.getWorkflowById(
        pathParams.workflow
    ).catch(noop);

    if (!workflow) throw redirect($path("/workflows"));

    const { threadId } = query ?? {};

    return { workflow, threadId };
};

export default function ChatAgent() {
    const { workflow, threadId } = useLoaderData<typeof clientLoader>();

    const navigate = useNavigate();

    return (
        <div className="h-full flex flex-col overflow-hidden relative">
            <ChatProvider
                id={workflow.id}
                mode="workflow"
                threadId={threadId}
                onCreateThreadId={(threadId) =>
                    navigate(
                        $path(
                            "/workflows/:workflow",
                            { workflow: workflow.id },
                            { threadId }
                        )
                    )
                }
            >
                <ResizablePanelGroup
                    direction="horizontal"
                    className="flex-auto"
                >
                    <ResizablePanel className="">
                        <Workflow workflow={workflow} />
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
