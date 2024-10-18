import { SquarePenIcon } from "lucide-react";
import { mutate } from "swr";

import { OAuthApp } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useOAuthAppSpec } from "~/hooks/oauthApps/useOAuthAppSpec";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";

type EditOAuthAppProps = {
    oauthApp: OAuthApp;
};

export function EditOAuthApp({ oauthApp }: EditOAuthAppProps) {
    const updateApp = useAsync(OauthAppService.updateOauthApp, {
        onSuccess: async () => {
            await mutate(OauthAppService.getOauthApps.key());
            modal.onClose();
        },
    });

    const modal = useDisclosure();

    const { data: spec } = useOAuthAppSpec();

    const typeSpec = spec.get(oauthApp.type);
    if (!typeSpec) return null;

    return (
        <Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
            <DialogTrigger asChild>
                <Button variant="ghost" size="icon">
                    <SquarePenIcon />
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogTitle>Edit OAuth App ({oauthApp.type})</DialogTitle>

                <DialogDescription hidden>
                    Update the OAuth app settings.
                </DialogDescription>

                <OAuthAppForm
                    appSpec={typeSpec}
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
