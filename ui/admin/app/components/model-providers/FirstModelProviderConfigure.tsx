import { BoxesIcon } from "lucide-react";
import { useState } from "react";

import { ModelProvider } from "~/lib/model/modelProviders";

import { ModelProviderConfigureContent } from "~/components/model-providers/ModelProviderConfigure";
import { useModelProviders } from "~/components/model-providers/ModelProviderContext";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";

export function FirstModelProviderConfigure({
    onSuccess,
}: {
    onSuccess: () => void;
}) {
    const { modelProviders } = useModelProviders();
    const [dialogIsOpen, setDialogIsOpen] = useState(false);

    const [selectedModelProvider, setSelectedModelProvider] =
        useState<ModelProvider | null>(null);

    const handleClose = () => {
        setDialogIsOpen(false);
        setSelectedModelProvider(null);
    };

    const handleOpenChange = (open: boolean) => {
        if (!open) {
            handleClose();
        }
    };

    const handleSuccessClose = () => {
        console.log("C");
        handleClose();
        onSuccess();
    };

    return (
        <Dialog open={dialogIsOpen} onOpenChange={handleOpenChange}>
            <DialogTrigger asChild>
                <Button
                    className="mt-0 w-fit px-10"
                    onClick={() => setDialogIsOpen(true)}
                >
                    Get Started
                </Button>
            </DialogTrigger>
            <DialogDescription hidden>
                Set up an initial model provider.
            </DialogDescription>

            {selectedModelProvider ? (
                <DialogContent
                    classNames={{
                        content: "transition-none",
                    }}
                >
                    <ModelProviderConfigureContent
                        modelProvider={selectedModelProvider}
                        onSuccess={handleSuccessClose}
                    />
                </DialogContent>
            ) : (
                <DialogContent
                    classNames={{
                        content: "max-w-xs",
                    }}
                >
                    <DialogHeader>
                        <DialogTitle>Select a Model Provider</DialogTitle>
                    </DialogHeader>

                    <div className="flex flex-col gap-4">
                        {modelProviders.map((modelProvider) => (
                            <Button
                                key={modelProvider.id}
                                onClick={() =>
                                    setSelectedModelProvider(modelProvider)
                                }
                                startContent={<BoxesIcon />}
                                variant="ghost"
                            >
                                {modelProvider.name}
                            </Button>
                        ))}
                    </div>
                </DialogContent>
            )}
        </Dialog>
    );
}
