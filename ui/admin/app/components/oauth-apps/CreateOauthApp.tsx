import { DialogDescription } from "@radix-ui/react-dialog";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { mutate } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { Button } from "~/components/ui/button";
import {
    Command,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
} from "~/components/ui/command";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { useOAuthAppSpec } from "~/hooks/oauthApps/useOAuthAppSpec";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

import { OAuthAppForm } from "./OAuthAppForm";

export function CreateOauthApp() {
    const selectModal = useDisclosure();
    const { data: spec } = useOAuthAppSpec();

    const [selectedAppKey, setSelectedAppKey] = useState<string | null>(null);

    const createApp = useAsync(OauthAppService.createOauthApp, {
        onSuccess: () => {
            mutate(OauthAppService.getOauthApps.key());
            setSelectedAppKey(null);
        },
    });

    const selectedSpec = selectedAppKey ? spec.get(selectedAppKey) : null;

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
                                {Array.from(spec.entries()).map(
                                    ([key, { displayName }]) => (
                                        <CommandItem
                                            key={key}
                                            value={displayName}
                                            onSelect={() => {
                                                setSelectedAppKey(key);
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
                    {selectedAppKey && selectedSpec && (
                        <>
                            <DialogTitle>
                                Create {selectedSpec.displayName} OAuth App
                            </DialogTitle>

                            <DialogDescription>
                                Create a new OAuth app for{" "}
                                {selectedSpec.displayName}
                            </DialogDescription>

                            <OAuthAppForm
                                appSpec={selectedSpec}
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
