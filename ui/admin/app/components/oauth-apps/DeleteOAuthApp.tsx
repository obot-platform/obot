import { TrashIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";

export function DeleteOAuthApp({
    id,
    disableTooltip,
}: {
    id: string;
    disableTooltip?: boolean;
}) {
    const deleteOAuthApp = useAsync(async () => {
        await OauthAppService.deleteOauthApp(id);
        await mutate(useOAuthAppList.key());

        toast.success("OAuth app deleted");
    });

    return (
        <div className="flex gap-2">
            <TooltipProvider>
                <Tooltip open={getIsOpen()}>
                    <ConfirmationDialog
                        title={`Delete OAuth App`}
                        description="Are you sure you want to delete this OAuth app?"
                        onConfirm={deleteOAuthApp.execute}
                        confirmProps={{
                            variant: "destructive",
                            children: "Delete",
                        }}
                    >
                        <TooltipTrigger asChild>
                            <Button
                                variant="destructive"
                                className="w-full"
                                disabled={deleteOAuthApp.isLoading}
                            >
                                {deleteOAuthApp.isLoading ? (
                                    <LoadingSpinner className="w-4 h-4 mr-2" />
                                ) : (
                                    <TrashIcon className="w-4 h-4 mr-2" />
                                )}
                                Delete OAuth App
                            </Button>
                        </TooltipTrigger>
                    </ConfirmationDialog>

                    <TooltipContent>Delete</TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );

    function getIsOpen() {
        if (disableTooltip) return false;
    }
}
