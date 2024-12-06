import { useEffect, useState } from "react";
import useSWR from "swr";

import { ModelProvider, ModelProviderConfig } from "~/lib/model/modelProviders";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { ModelProviderForm } from "~/components/model-providers/ModelProviderForm";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { DefaultModelAliasForm } from "~/components/model/shared/DefaultModelAliasForm";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";

type ModelProviderConfigureProps = {
    modelProvider: ModelProvider;
};

export function ModelProviderConfigure({
    modelProvider,
}: ModelProviderConfigureProps) {
    const [dialogIsOpen, setDialogIsOpen] = useState(false);
    const [showDefaultModelAliasForm, setShowDefaultModelAliasForm] =
        useState(false);

    const [loadingModelProviderId, setLoadingModelProviderId] = useState("");

    const getLoadingModelProviderModels = useSWR(
        loadingModelProviderId
            ? ModelProviderApiService.getModelProviderById.key(
                  loadingModelProviderId
              )
            : null,
        ({ modelProviderId }) =>
            ModelProviderApiService.getModelProviderById(modelProviderId),
        {
            revalidateOnFocus: false,
            refreshInterval: 2000,
        }
    );

    useEffect(() => {
        if (!loadingModelProviderId) return;

        const { isLoading, data } = getLoadingModelProviderModels;
        if (isLoading) return;

        if (data?.modelsBackPopulated) {
            setShowDefaultModelAliasForm(true);
            setLoadingModelProviderId("");
        }
    }, [getLoadingModelProviderModels, loadingModelProviderId]);

    const handleDone = () => {
        setDialogIsOpen(false);
        setShowDefaultModelAliasForm(false);
    };

    return (
        <Dialog open={dialogIsOpen} onOpenChange={setDialogIsOpen}>
            <DialogTrigger asChild>
                <Button
                    variant={modelProvider.configured ? "secondary" : "accent"}
                    className="mt-0 w-full"
                >
                    {modelProvider.configured ? "Modify" : "Configure"}
                </Button>
            </DialogTrigger>

            <DialogDescription hidden>
                Configure Model Provider
            </DialogDescription>

            <DialogContent
                className="p-0 gap-0"
                hideCloseButton={loadingModelProviderId !== ""}
            >
                {loadingModelProviderId ? (
                    <div className="flex items-center justify-center gap-1 p-2">
                        <LoadingSpinner /> Loading {modelProvider.name}{" "}
                        Models...
                    </div>
                ) : showDefaultModelAliasForm ? (
                    <div className="p-6">
                        <DialogHeader>
                            <DialogTitle className="flex items-center gap-2 pb-4">
                                Configure Default Model Aliases
                            </DialogTitle>
                        </DialogHeader>
                        <DefaultModelAliasForm onSuccess={handleDone} />
                    </div>
                ) : (
                    <ModelProviderConfigureContent
                        modelProvider={modelProvider}
                        onSuccess={() =>
                            setLoadingModelProviderId(modelProvider.id)
                        }
                    />
                )}
            </DialogContent>
        </Dialog>
    );
}

export function ModelProviderConfigureContent({
    modelProvider,
    onSuccess,
}: {
    modelProvider: ModelProvider;
    onSuccess: () => void;
}) {
    const revealModelProvider = useSWR(
        ModelProviderApiService.revealModelProviderById.key(modelProvider.id),
        ({ modelProviderId }) =>
            ModelProviderApiService.revealModelProviderById(modelProviderId),
        { keepPreviousData: true }
    );

    const handleSuccess = (config: ModelProviderConfig) => {
        revealModelProvider.mutate(config, false);
        onSuccess();
    };

    const requiredParameters = modelProvider.requiredConfigurationParameters;
    const parameters = revealModelProvider.data;

    return (
        <>
            <DialogHeader className="space-y-0">
                <DialogTitle className="flex items-center gap-2 px-6 py-4">
                    <ModelProviderIcon modelProvider={modelProvider} />{" "}
                    {modelProvider.configured
                        ? `Configure ${modelProvider.name}`
                        : `Set Up ${modelProvider.name}`}
                </DialogTitle>
            </DialogHeader>
            {revealModelProvider.isLoading ? (
                <LoadingSpinner />
            ) : (
                <ModelProviderForm
                    modelProvider={modelProvider}
                    onSuccess={handleSuccess}
                    parameters={parameters ?? {}}
                    requiredParameters={requiredParameters ?? []}
                />
            )}
        </>
    );
}
