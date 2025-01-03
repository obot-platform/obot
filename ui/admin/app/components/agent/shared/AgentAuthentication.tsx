import { ScanEyeIcon, UserRoundCheckIcon } from "lucide-react";

import { ToolInfo } from "~/lib/model/agents";
import { AssistantNamespace } from "~/lib/model/assistants";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { ToolAuthApiService } from "~/lib/service/api/toolAuthApiService";

import { useToolReference } from "~/components/agent/ToolEntry";
import { AgentAuthenticationDialog } from "~/components/agent/shared/AgentAuthenticationDialog";
import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useToolAuthPolling } from "~/hooks/toolAuth/useToolAuthPolling";
import { useAsync } from "~/hooks/useAsync";

type AgentAuthenticationProps = {
    tool: string;
    toolInfo?: ToolInfo;
    entityId: string;
    onUpdate: (toolInfo: ToolInfo) => void;
    namespace: AssistantNamespace;
};

export function AgentAuthentication({
    tool,
    entityId,
    onUpdate,
    namespace,
}: AgentAuthenticationProps) {
    const authorize = useAsync(ToolAuthApiService.authenticateTools);
    const deauthorize = useAsync(ToolAuthApiService.deauthenticateTools);
    const cancelAuthorize = useAsync(ThreadsService.abortThread);

    const { threadId, reader } = authorize.data ?? {};

    const { toolInfo, isPolling } = useToolAuthPolling(namespace, entityId);

    const { credentialNames, authorized } = toolInfo?.[tool] ?? {};

    const { interceptAsync, dialogProps } = useConfirmationDialog();

    const handleAuthorize = async () => {
        authorize.execute(namespace, entityId, [tool]);
    };

    const handleDeauthorize = async () => {
        if (!toolInfo) return;

        const { error } = await deauthorize.executeAsync(namespace, entityId, [
            tool,
        ]);

        if (error) return;

        onUpdate({ ...toolInfo, authorized: false });
    };

    const handleAuthorizeComplete = () => {
        if (!threadId) {
            console.error(new Error("Thread ID is undefined"));
            return;
        } else {
            reader?.cancel();
            cancelAuthorize.execute(threadId);
        }

        authorize.clear();
        onUpdate({ ...toolInfo, authorized: true });
    };

    const handleClick = authorized
        ? () => interceptAsync(handleDeauthorize)
        : handleAuthorize;

    const loading = authorize.isLoading || cancelAuthorize.isLoading;

    const { icon, label } = useToolReference(tool);

    if (isPolling)
        return (
            <Tooltip>
                <TooltipContent>Authentication Processing</TooltipContent>

                <TooltipTrigger asChild>
                    <Button size="icon" variant="ghost" loading />
                </TooltipTrigger>
            </Tooltip>
        );

    if (!credentialNames?.length) return null;

    return (
        <>
            <Tooltip>
                <TooltipTrigger asChild>
                    <Button
                        size="icon"
                        variant="ghost"
                        onClick={handleClick}
                        loading={loading}
                    >
                        {authorized ? <UserRoundCheckIcon /> : <ScanEyeIcon />}
                    </Button>
                </TooltipTrigger>

                <TooltipContent>
                    {authorized ? "Remove authorization" : "Authorize tool"}
                </TooltipContent>
            </Tooltip>

            <AgentAuthenticationDialog
                tool={tool}
                entityId={entityId}
                threadId={threadId}
                onComplete={handleAuthorizeComplete}
            />

            <ConfirmationDialog
                {...dialogProps}
                title={
                    <span className="flex items-center gap-2">
                        <span>{icon}</span>
                        <span>Remove Authentication?</span>
                    </span>
                }
                description={`Are you sure you want to remove authentication for ${label}? this will require each thread to re-authenticate in order to use this tool.`}
                confirmProps={{
                    variant: "destructive",
                    children: "Delete Authentication",
                }}
            />
        </>
    );
}
