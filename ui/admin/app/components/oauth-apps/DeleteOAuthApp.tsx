import { Check, TrashIcon, XIcon } from "lucide-react";
import { mutate } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

export function DeleteOAuthApp({ id }: { id: string }) {
    const confirmation = useDisclosure();

    const deleteOAuthApp = useAsync(OauthAppService.deleteOauthApp, {
        onSuccess: () => mutate(OauthAppService.getOauthApps.key()),
    });

    return (
        <div className="flex gap-2">
            {confirmation.isOpen ? (
                <>
                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Button
                                    variant="secondary"
                                    size="icon"
                                    onClick={confirmation.onClose}
                                >
                                    <XIcon />
                                </Button>
                            </TooltipTrigger>

                            <TooltipContent>Cancel</TooltipContent>
                        </Tooltip>
                    </TooltipProvider>

                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Button
                                    variant="destructive"
                                    size="icon"
                                    disabled={deleteOAuthApp.isLoading}
                                    onClick={() => {
                                        deleteOAuthApp.execute(id);
                                        confirmation.onClose();
                                    }}
                                >
                                    <Check />
                                </Button>
                            </TooltipTrigger>

                            <TooltipContent>Confirm Delete</TooltipContent>
                        </Tooltip>
                    </TooltipProvider>
                </>
            ) : (
                <>
                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Button
                                    variant="destructive"
                                    size="icon"
                                    disabled={deleteOAuthApp.isLoading}
                                    onClick={confirmation.onOpen}
                                >
                                    {deleteOAuthApp.isLoading ? (
                                        <LoadingSpinner />
                                    ) : (
                                        <TrashIcon />
                                    )}
                                </Button>
                            </TooltipTrigger>

                            <TooltipContent>Delete</TooltipContent>
                        </Tooltip>
                    </TooltipProvider>

                    <Button className="invisible" size="icon" />
                </>
            )}
        </div>
    );
}
