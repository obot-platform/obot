import { ScanEyeIcon, UserRoundCheckIcon } from "lucide-react";

import { ToolInfo } from "~/lib/model/agents";
import { AssistantNamespace } from "~/lib/model/assistants";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { ToolAuthApiService } from "~/lib/service/api/toolAuthApiService";

import { useToolReference } from "~/components/agent/ToolEntry";
import { ToolAuthenticationDialog } from "~/components/agent/shared/ToolAuthenticationDialog";
import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
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

export function ToolAuthenticationStatus({
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
                <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                        <TooltipTrigger asChild>
                            <Button
                                size="icon"
                                variant="ghost"
                                loading={loading}
                            >
                                {authorized ? (
                                    <UserRoundCheckIcon />
                                ) : (
                                    <ScanEyeIcon />
                                )}
                            </Button>
                        </TooltipTrigger>
                    </DropdownMenuTrigger>

                    <DropdownMenuContent side="right" align="start">
                        <DropdownMenuLabel>
                            {authorized ? "Authorized" : "Unauthorized"}
                        </DropdownMenuLabel>

                        {authorized ? (
                            <DropdownMenuItem
                                variant="destructive"
                                onClick={() =>
                                    interceptAsync(handleDeauthorize)
                                }
                            >
                                Remove Authorization
                            </DropdownMenuItem>
                        ) : (
                            <DropdownMenuItem onClick={handleAuthorize}>
                                Authorize Tool
                            </DropdownMenuItem>
                        )}
                    </DropdownMenuContent>
                </DropdownMenu>

                <TooltipContent>Authorization Status</TooltipContent>
            </Tooltip>

            <ToolAuthenticationDialog
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
