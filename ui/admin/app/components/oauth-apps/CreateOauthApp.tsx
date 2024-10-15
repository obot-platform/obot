import { DialogDescription } from "@radix-ui/react-dialog";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { mutate } from "swr";

import { OAuthAppSpec, OAuthAppType } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import {
    Command,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
} from "../ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { OAuthAppForm } from "./OAuthAppForm";

type CreateOauthAppProps = {
    spec: OAuthAppSpec;
};

export function CreateOauthApp({ spec }: CreateOauthAppProps) {
    const selectModal = useDisclosure();

    const [selectedAppKey, setSelectedAppKey] = useState<OAuthAppType | null>(
        null
    );

    const createApp = useAsync(OauthAppService.createOauthApp, {
        onSuccess: () => {
            mutate(OauthAppService.getOauthApps.key());
            setSelectedAppKey(null);
        },
    });

    return (
        <>
            <Popover
                open={selectModal.isOpen}
                onOpenChange={selectModal.onOpenChange}
            >
                <PopoverTrigger asChild>
                    <Button variant="outline">
                        <PlusIcon className="w-4 h-4 mr-2" />
                        New OAuth App
                    </Button>
                </PopoverTrigger>

                <PopoverContent className="p-0" side="bottom" align="end">
                    <Command>
                        <CommandInput placeholder="Search OAuth App..." />

                        <CommandList>
                            <CommandGroup>
                                {Object.entries(spec).map(
                                    ([key, { displayName }]) => (
                                        <CommandItem
                                            key={key}
                                            value={displayName}
                                            onSelect={() => {
                                                setSelectedAppKey(
                                                    key as OAuthAppType
                                                );
                                                selectModal.onClose();
                                            }}
                                        >
                                            {displayName}
                                        </CommandItem>
                                    )
                                )}
                            </CommandGroup>
                        </CommandList>
                    </Command>
                </PopoverContent>
            </Popover>

            <Dialog
                open={!!selectedAppKey}
                onOpenChange={() => setSelectedAppKey(null)}
            >
                <DialogContent>
                    {selectedAppKey && spec[selectedAppKey] && (
                        <>
                            <DialogTitle>
                                Create {spec[selectedAppKey].displayName} OAuth
                                App
                            </DialogTitle>

                            <DialogDescription>
                                Create a new OAuth app for{" "}
                                {spec[selectedAppKey].displayName}
                            </DialogDescription>

                            <OAuthAppForm
                                appSpec={spec[selectedAppKey]}
                                onSubmit={(data) =>
                                    createApp.execute({
                                        type: selectedAppKey,
                                        ...data,
                                    })
                                }
                            />
                        </>
                    )}
                </DialogContent>
            </Dialog>
        </>
    );
}
