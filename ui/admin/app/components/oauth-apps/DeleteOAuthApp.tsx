import { TrashIcon } from "lucide-react";
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
import { useAsync } from "~/hooks/useAsync";

export function DeleteOAuthApp({ id }: { id: string }) {
    const deleteOAuthApp = useAsync(OauthAppService.deleteOauthApp, {
        onSuccess: () => mutate(OauthAppService.getOauthApps.key()),
    });

    return (
        <div className="flex gap-2">
            <TooltipProvider>
                <Tooltip>
                    <ConfirmationDialog
                        title={`Delete OAuth App`}
                        description="Are you sure you want to delete this OAuth app?"
                        onConfirm={() => deleteOAuthApp.execute(id)}
                        confirmProps={{
                            variant: "destructive",
                            children: "Delete",
                        }}
                    >
                        <TooltipTrigger asChild>
                            <Button
                                variant="ghost"
                                size="icon"
                                disabled={deleteOAuthApp.isLoading}
                            >
                                {deleteOAuthApp.isLoading ? (
                                    <LoadingSpinner />
                                ) : (
                                    <TrashIcon />
                                )}
                            </Button>
                        </TooltipTrigger>
                    </ConfirmationDialog>

                    <TooltipContent>Delete</TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );
}
