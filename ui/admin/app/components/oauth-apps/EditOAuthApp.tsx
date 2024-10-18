import { SquarePenIcon } from "lucide-react";
import { mutate } from "swr";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useOAuthAppInfo } from "~/hooks/oauthApps";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function EditOAuthApp({ type }: { type: OAuthProvider }) {
    const spec = useOAuthAppInfo(type);
    const modal = useDisclosure();

    const updateApp = useAsync(OauthAppService.updateOauthApp, {
        onSuccess: async () => {
            await mutate(OauthAppService.getOauthApps.key());
            modal.onClose();
        },
    });

    const { customApp } = spec;

    if (!customApp) return null;

    return (
        <TooltipProvider>
            <Tooltip>
                <Dialog>
                    <DialogTrigger asChild>
                        <TooltipTrigger asChild>
                            <Button variant="ghost" size="icon">
                                <SquarePenIcon />
                            </Button>
                        </TooltipTrigger>
                    </DialogTrigger>

                    <DialogContent>
                        <DialogTitle>
                            <OAuthAppTypeIcon type={type} /> Edit{" "}
                            {spec.displayName} OAuth Configuration
                        </DialogTitle>

                        <DialogDescription hidden>
                            Update the OAuth app settings.
                        </DialogDescription>

                        <OAuthAppForm
                            type={type}
                            onSubmit={(data) =>
                                updateApp.execute(customApp.id, {
                                    type: customApp.type,
                                    refName: customApp.refName,
                                    ...data,
                                })
                            }
                        />
                    </DialogContent>
                </Dialog>

                <TooltipContent side="bottom">Edit</TooltipContent>
            </Tooltip>
        </TooltipProvider>
    );
}
