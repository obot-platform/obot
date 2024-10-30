import { SettingsIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthAppParams } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function ConfigureOAuthApp({ type }: { type: OAuthProvider }) {
    const spec = useOAuthAppInfo(type);
    const { appOverride } = spec;
    const isEdit = !!appOverride;

    const modal = useDisclosure();
    const successModal = useDisclosure();

    const createApp = useAsync(async (data: OAuthAppParams) => {
        await OauthAppService.createOauthApp({
            ...data,
            type,
            refName: type,
            global: true,
            integration: type,
        });

        await mutate(OauthAppService.getOauthApps.key());

        modal.onClose();
        successModal.onOpen();
        toast.success(`${spec.displayName} OAuth configuration created`);
    });

    const updateApp = useAsync(async (data: OAuthAppParams) => {
        if (!appOverride) throw new Error("Custom app not found");

        await OauthAppService.updateOauthApp(appOverride.id, {
            ...data,
            type: appOverride.type,
            refName: appOverride.refName,
            global: appOverride.global,
            integration: appOverride.integration,
        });

        await mutate(OauthAppService.getOauthApps.key());

        modal.onClose();
        successModal.onOpen();
        toast.success(`${spec.displayName} OAuth configuration updated`);
    });

    return (
        <>
            <Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
                <DialogTrigger asChild>
                    <Button className="w-full">
                        <SettingsIcon className="w-4 h-4 mr-2" />
                        {isEdit
                            ? "Replace Configuration"
                            : `Configure ${spec.displayName} OAuth App`}
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
                            onSubmit={
                                isEdit ? updateApp.execute : createApp.execute
                            }
                            isLoading={
                                isEdit
                                    ? updateApp.isLoading
                                    : createApp.isLoading
                            }
                        />
                    </ScrollArea>
                </DialogContent>
            </Dialog>

            <Dialog
                open={successModal.isOpen}
                onOpenChange={successModal.onOpenChange}
            >
                <DialogContent>
                    <DialogTitle>
                        Successfully Configured {spec.displayName} OAuth App
                    </DialogTitle>

                    <DialogDescription>
                        Otto will now use your custom {spec.displayName} OAuth
                        app to authenticate users.
                    </DialogDescription>

                    <DialogFooter>
                        <DialogClose asChild>
                            <Button className="w-full">Close</Button>
                        </DialogClose>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </>
    );
}
