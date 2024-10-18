import { DialogDescription } from "@radix-ui/react-dialog";
import { SettingsIcon } from "lucide-react";
import { mutate } from "swr";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useOAuthAppInfo } from "~/hooks/oauthApps";
import { useAsync } from "~/hooks/useAsync";

import { ScrollArea } from "../ui/scroll-area";
import { OAuthAppForm } from "./OAuthAppForm";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function CreateOauthApp({ type }: { type: OAuthProvider }) {
    const spec = useOAuthAppInfo(type);

    const createApp = useAsync(OauthAppService.createOauthApp, {
        onSuccess: () => mutate(OauthAppService.getOauthApps.key()),
    });

    return (
        <Dialog>
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
                        onSubmit={(data) =>
                            createApp.execute({
                                type,
                                refName: type,
                                ...data,
                            })
                        }
                    />
                </ScrollArea>
            </DialogContent>
        </Dialog>
    );
}
