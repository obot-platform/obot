import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR, { mutate } from "swr";
import { z } from "zod";

import {
    Model,
    ModelManifest,
    ModelManifestSchema,
    ModelProvider,
    ModelUsage,
    getModelUsageLabel,
    getModelsForProvider,
} from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { ControlledCustomInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";

type ModelFormProps = {
    model?: Model;
    onSubmit: (model: ModelManifest) => void;
};

type FormValues = z.infer<typeof ModelManifestSchema>;

export function ModelForm(props: ModelFormProps) {
    const { model, onSubmit } = props;

    const { data: modelProviders } = useSWR(
        ModelApiService.getModelProviders.key(),
        ModelApiService.getModelProviders
    );

    const updateModel = useAsync(ModelApiService.updateModel, {
        onSuccess: (values) => {
            toast.success("Model updated");
            mutate(ModelApiService.getModels.key());
            onSubmit(values);
        },
        onError,
    });

    const createModel = useAsync(ModelApiService.createModel, {
        onSuccess: (values) => {
            toast.success("Model created");
            mutate(ModelApiService.getModels.key());
            onSubmit(values);
        },
        onError,
    });

    const defaultValues = useMemo<FormValues>(() => {
        return {
            name: model?.name ?? "",
            targetModel: model?.targetModel ?? "",
            modelProvider: model?.modelProvider ?? "",
            active: model?.active ?? true,
            usage: model?.usage ?? ModelUsage.LLM,
        };
    }, [model]);

    const form = useForm<FormValues>({
        resolver: zodResolver(ModelManifestSchema),
        defaultValues,
    });

    const { loading, submit } = getSubmitInfo();

    const handleSubmit = form.handleSubmit((values) =>
        submit({ ...values, name: values.targetModel })
    );

    const providerName = (provider: ModelProvider) => {
        let text = provider.name || provider.id;

        if (!provider.modelProviderStatus.configured)
            text += " (not configured)";

        return text;
    };

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="space-y-4">
                <ControlledCustomInput
                    control={form.control}
                    name="modelProvider"
                    label="Model Provider"
                >
                    {({ field: { ref: _, ...field }, className }) => (
                        <Select {...field} onValueChange={field.onChange}>
                            <SelectTrigger className={className}>
                                <SelectValue placeholder="Select a model provider" />
                            </SelectTrigger>

                            <SelectContent>
                                {modelProviders?.map((provider) => (
                                    <SelectItem
                                        key={provider.id}
                                        value={provider.id}
                                        disabled={
                                            !provider.modelProviderStatus
                                                .configured
                                        }
                                    >
                                        {providerName(provider)}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    )}
                </ControlledCustomInput>

                <ControlledCustomInput
                    control={form.control}
                    name="targetModel"
                    label="Target Model"
                >
                    {({ field: { ref: _, ...field }, className }) => {
                        const models = getModelsForProvider(
                            form.watch("modelProvider")
                        );

                        return (
                            <Select
                                {...field}
                                disabled={!form.watch("modelProvider")}
                                onValueChange={field.onChange}
                            >
                                <SelectTrigger className={className}>
                                    <SelectValue placeholder="Select a Model" />
                                </SelectTrigger>

                                <SelectContent>
                                    {models.map((model) => (
                                        <SelectItem key={model} value={model}>
                                            {model}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>
                        );
                    }}
                </ControlledCustomInput>

                <ControlledCustomInput
                    control={form.control}
                    name="usage"
                    label="Usage"
                >
                    {({ field: { ref: _, ...field }, className }) => (
                        <Select {...field} onValueChange={field.onChange}>
                            <SelectTrigger className={className}>
                                <SelectValue placeholder="Select Usage" />
                            </SelectTrigger>

                            <SelectContent>
                                {Object.entries(ModelUsage).map(
                                    ([key, value]) => (
                                        <SelectItem key={key} value={value}>
                                            {getModelUsageLabel(value)}
                                        </SelectItem>
                                    )
                                )}
                            </SelectContent>
                        </Select>
                    )}
                </ControlledCustomInput>

                <Button
                    type="submit"
                    className="w-full"
                    disabled={loading}
                    loading={loading}
                >
                    Submit
                </Button>
            </form>
        </Form>
    );

    function getSubmitInfo() {
        if (model) {
            return {
                isEdit: true,
                loading: updateModel.isLoading,
                submit: (values: FormValues) =>
                    updateModel.execute(model.id, values),
            };
        }

        return {
            isEdit: false,
            loading: createModel.isLoading,
            submit: (values: FormValues) => createModel.execute(values),
        };
    }

    function onError(error: unknown) {
        if (error instanceof Error) toast.error(error.message);
        else toast.error("Model failed to save.");
    }
}
