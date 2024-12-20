import { useEffect, useState } from "react";
import useSWR, { mutate } from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { NotFoundError } from "~/lib/service/api/apiErrors";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { ModelProviderForm } from "~/components/model-providers/ModelProviderForm";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { CommonModelProviderIds } from "~/components/model-providers/constants";
import { DefaultModelAliasForm } from "~/components/model/DefaultModelAliasForm";
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
import { Link } from "~/components/ui/link";

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
        ModelProviderApiService.getModelProviderById.key(
            loadingModelProviderId
        ),
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
            // revalidate models to get back populated models
            mutate(ModelApiService.getModels.key());
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
                className="p-0 gap-0 max-w-2xl"
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
                        <DialogDescription>
                            When no model is specified, a default model is used
                            for creating a new agent, workflow, or working with
                            some tools, etc. Select your default models for the
                            usage types below.
                        </DialogDescription>
                        <div className="mt-4">
                            <DefaultModelAliasForm onSuccess={handleDone} />
                        </div>
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
        {
            keepPreviousData: true,
            // 404: no credential found = no need to retry
            shouldRetryOnError: (error) =>
                error instanceof NotFoundError &&
                error.message.toLowerCase().includes("no credential found")
                    ? false
                    : true,
        }
    );

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

            {(modelProvider.id === CommonModelProviderIds.ANTHROPIC ||
                modelProvider.id ==
                    CommonModelProviderIds.ANTHROPIC_BEDROCK) && (
                <DialogDescription className="px-4">
                    Note: Anthropic does not have an embeddings model and{" "}
                    <Link
                        target="_blank"
                        rel="noreferrer"
                        to="https://docs.anthropic.com/en/docs/build-with-claude/embeddings"
                    >
                        recommends
                    </Link>{" "}
                    Voyage AI.
                </DialogDescription>
            )}
            {revealModelProvider.isLoading ? (
                <LoadingSpinner />
            ) : (
                <ModelProviderForm
                    modelProvider={modelProvider}
                    onSuccess={onSuccess}
                    parameters={parameters ?? {}}
                    requiredParameters={requiredParameters ?? []}
                />
            )}
        </>
    );
}
