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
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function ConfigureOAuthApp({
    type,
    onSuccess,
}: {
    type: OAuthProvider;
    onSuccess: () => void;
}) {
    const spec = useOAuthAppInfo(type);
    const { appOverride } = spec;
    const isEdit = !!appOverride;

    const modal = useDisclosure();

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
        toast.success(`${spec.displayName} OAuth configuration created`);
        onSuccess();
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
        toast.success(`${spec.displayName} OAuth configuration updated`);
        onSuccess();
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
                    className="lg:max-w-3xl"
                    classNames={{
                        overlay: "opacity-0",
                    }}
                    aria-describedby="create-oauth-app"
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
        </>
    );
}
