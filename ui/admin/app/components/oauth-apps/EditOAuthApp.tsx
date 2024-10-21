import { GearIcon } from "@radix-ui/react-icons";
import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthAppParams } from "~/lib/model/oauthApps";
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
    useOAuthAppInfo,
    useOAuthAppList,
} from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function EditOAuthApp({ type }: { type: OAuthProvider }) {
    const spec = useOAuthAppInfo(type);
    const modal = useDisclosure();

    const { customApp } = spec;

    const updateApp = useAsync(async (data: OAuthAppParams) => {
        if (!customApp) return;

        await OauthAppService.updateOauthApp(customApp.id, {
            type: customApp.type,
            refName: customApp.refName,
            ...data,
        });
        await mutate(useOAuthAppList.key());
        modal.onClose();
        toast.success(`${spec.displayName} OAuth app updated`);
    });

    if (!customApp) return null;

    return (
        <Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
            <DialogTrigger asChild>
                <Button>
                    <GearIcon className="w-4 h-4 mr-2" />
                    Edit Configuration
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogTitle className="flex items-center gap-2">
                    <OAuthAppTypeIcon type={type} /> Edit {spec.displayName}{" "}
                    OAuth Configuration
                </DialogTitle>

                <DialogDescription hidden>
                    Update the OAuth app settings.
                </DialogDescription>

                <OAuthAppForm
                    type={type}
                    onSubmit={updateApp.execute}
                    isLoading={updateApp.isLoading}
                />
            </DialogContent>
        </Dialog>
    );
}
