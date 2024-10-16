import { PencilIcon } from "lucide-react";
import { mutate } from "swr";

import { OAuthApp, OAuthAppSpec } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { noop } from "~/lib/utils";

import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { Button } from "../ui/button";
import { OAuthAppForm } from "./OAuthAppForm";

type EditOAuthAppProps = {
    oauthApp: OAuthApp;
    appSpec: OAuthAppSpec;
};

export function EditOAuthApp({ oauthApp, appSpec }: EditOAuthAppProps) {
    const updateApp = useAsync(OauthAppService.updateOauthApp, {
        onSuccess: async () => {
            await mutate(OauthAppService.getOauthApps.key());
            modal.onClose();
        },
    });

    const modal = useDisclosure();

    return (
        <Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
            <DialogTrigger asChild>
                <Button variant="ghost" size="icon">
                    <PencilIcon />
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogTitle>Edit OAuth App ({oauthApp.type})</DialogTitle>

                <DialogDescription hidden>
                    Update the OAuth app settings.
                </DialogDescription>

                <OAuthAppForm
                    appSpec={appSpec[oauthApp.type]}
                    oauthApp={oauthApp}
                    onSubmit={(data) =>
                        updateApp.execute(oauthApp.id, {
                            type: oauthApp.type,
                            ...data,
                        })
                    }
                />
            </DialogContent>
        </Dialog>
    );
}
