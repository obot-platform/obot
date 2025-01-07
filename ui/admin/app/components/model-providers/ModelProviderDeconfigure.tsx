import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { DropdownMenuItem } from "~/components/ui/dropdown-menu";
import { useAsync } from "~/hooks/useAsync";

export function ModelProviderDeconfigure({
    modelProvider,
}: {
    modelProvider: ModelProvider;
}) {
    const [open, setOpen] = useState(false);
    const handleDeconfigure = async () => {
        deconfigure.execute(modelProvider.id);
    };

    const deconfigure = useAsync(
        ModelProviderApiService.deconfigureModelProviderById,
        {
            onSuccess: () => {
                toast.success(`${modelProvider.name} deconfigured.`);
                mutate(ModelProviderApiService.getModelProviders.key());
                mutate(ModelApiService.getModels.key());
            },
            onError: () =>
                toast.error(`Failed to deconfigure ${modelProvider.name}`),
        }
    );

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger>
                <DropdownMenuItem
                    onClick={(event) => {
                        event.preventDefault();
                        setOpen(true);
                    }}
                    className="text-destructive"
                >
                    Deconfigure Model Provider
                </DropdownMenuItem>
            </DialogTrigger>

            <DialogDescription hidden>
                Configure Model Provider
            </DialogDescription>

            <DialogContent hideCloseButton>
                <DialogHeader>
                    <DialogTitle>Deconfigure {modelProvider.name}</DialogTitle>
                </DialogHeader>
                <p>
                    Deconfiguring this model provider will remove all models
                    associated with it and reset it to its unconfigured state.
                    You will need to set up the model provider once again to use
                    it.
                </p>

                <p>
                    Are you sure you want to deconfigure{" "}
                    <b>{modelProvider.name}</b>?
                </p>

                <DialogFooter>
                    <div className="w-full flex justify-center items-center gap-10 pt-4">
                        <DialogClose asChild>
                            <Button className="w-1/2" variant="outline">
                                Cancel
                            </Button>
                        </DialogClose>
                        <DialogClose asChild>
                            <Button
                                className="w-1/2"
                                onClick={handleDeconfigure}
                                variant="destructive"
                            >
                                Confirm
                            </Button>
                        </DialogClose>
                    </div>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
