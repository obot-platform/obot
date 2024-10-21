import { DialogDescription } from "@radix-ui/react-dialog";
import { SettingsIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthAppParams } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    useOAuthAppInfo,
    useOAuthAppList,
} from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function CreateOauthApp({ type }: { type: OAuthProvider }) {
    const spec = useOAuthAppInfo(type);
    const modal = useDisclosure();

    const createApp = useAsync(async (data: OAuthAppParams) => {
        await OauthAppService.createOauthApp({
            type,
            refName: type,
            ...data,
        });

        await mutate(useOAuthAppList.key());

        modal.onClose();
        toast.success(`${spec.displayName} OAuth app created`);
    });

    return (
        <Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
            <DialogTrigger asChild>
                <Button className="w-full">
                    <SettingsIcon className="w-4 h-4 mr-2" />
                    Configure {spec.displayName} OAuth App
                </Button>
            </DialogTrigger>

            <DialogContent
                classNames={{
                    overlay: "opacity-0",
                }}
                aria-describedby="create-oauth-app"
                className="px-0"
            >
                <DialogTitle className="flex items-center gap-2 px-4">
                    <OAuthAppTypeIcon type={type} />
                    Configure {spec.displayName} OAuth App
                </DialogTitle>

                <DialogDescription hidden>
                    Create a new OAuth app for {spec.displayName}
                </DialogDescription>

                <ScrollArea className="max-h-[80vh] px-4">
                    <OAuthAppForm
                        type={type}
                        onSubmit={createApp.execute}
                        isLoading={createApp.isLoading}
                    />
                </ScrollArea>
            </DialogContent>
        </Dialog>
    );
}
