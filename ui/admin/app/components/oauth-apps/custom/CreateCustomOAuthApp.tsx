import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { mutate } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useAsync } from "~/hooks/useAsync";

import { CustomOAuthAppForm } from "./CustomOAuthAppForm";

export function CreateCustomOAuthApp() {
    const [isOpen, setIsOpen] = useState(false);

    const createApp = useAsync(OauthAppService.createOauthApp, {
        onSuccess: () => {
            setIsOpen(false);
            mutate(OauthAppService.getOauthApps.key());
        },
    });

    return (
        <Dialog open={isOpen} onOpenChange={setIsOpen}>
            <DialogTrigger asChild>
                <Button variant="outline" className="flex items-center gap-2">
                    <PlusIcon className="h-4 w-4" /> Create a Custom OAuth App
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Create Custom OAuth App</DialogTitle>
                </DialogHeader>

                <DialogDescription hidden>
                    Create Custom OAuth App
                </DialogDescription>

                <CustomOAuthAppForm
                    onSubmit={(data) =>
                        createApp.execute({
                            type: "custom",
                            global: true,
                            refName: data.integration,
                            ...data,
                        })
                    }
                />
            </DialogContent>
        </Dialog>
    );
}
