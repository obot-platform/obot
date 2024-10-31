import { PenBoxIcon } from "lucide-react";
import { useState } from "react";
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

import { CustomOAuthAppForm } from "./CustomOAuthAppForm";

type EditCustomOAuthAppProps = {
    app: OAuthApp;
};

export function EditCustomOAuthApp({ app }: EditCustomOAuthAppProps) {
    const [isOpen, setIsOpen] = useState(false);

    return (
        <Dialog open={isOpen} onOpenChange={setIsOpen}>
            <DialogTrigger asChild>
                <Button variant="ghost" size="icon">
                    <PenBoxIcon />
                </Button>
            </DialogTrigger>

            <DialogDescription hidden>Edit Custom OAuth App</DialogDescription>

            <DialogContent>
                <DialogTitle>Edit Custom OAuth App</DialogTitle>

                <CustomOAuthAppForm
                    defaultData={app}
                    onComplete={() => {
                        mutate(OauthAppService.getOauthApps.key());
                        setIsOpen(false);
                    }}
                    onCancel={() => setIsOpen(false)}
                />
            </DialogContent>
        </Dialog>
    );
}
