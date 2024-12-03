import { zodResolver } from "@hookform/resolvers/zod";
import { BoxesIcon, PlusIcon, SettingsIcon, TrashIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import useSWR from "swr";
import { z } from "zod";

import { ModelProvider } from "~/lib/model/modelProviders";
import { ModelProviderService } from "~/lib/service/api/modelProviderApiService";

import { ControlledInput } from "~/components/form/controlledInputs";
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
import { Form } from "~/components/ui/form";
import { Separator } from "~/components/ui/separator";
import { useAsync } from "~/hooks/useAsync";

type ModelProviderConfigureProps = {
    variant: "cog" | "button";
    modelProvider: ModelProvider;
};

const formSchema = z.object({
    requiredConfigParams: z.array(
        z.object({
            name: z.string().min(1, {
                message: "Name is required.",
            }),
            value: z.string().min(1, {
                message: "This field is required.",
            }),
        })
    ),
    additionalConfirmParams: z.array(
        z.object({
            name: z.string(),
            value: z.string(),
        })
    ),
});

export type ModelProviderFormValues = z.infer<typeof formSchema>;

export function ModelProviderConfigure({
    variant,
    modelProvider,
}: ModelProviderConfigureProps) {
    const [dialogIsOpen, setDialogIsOpen] = useState(false);
    const handleOpenChange = (open: boolean) => {
        if (!open) {
            setDialogIsOpen(false);
        }
    };

    return (
        <Dialog open={dialogIsOpen} onOpenChange={handleOpenChange}>
            {variant === "cog" ? (
                <DialogTrigger asChild>
                    <Button
                        size="icon"
                        variant="ghost"
                        className="mt-0"
                        onClick={() => setDialogIsOpen(true)}
                    >
                        <SettingsIcon />
                    </Button>
                </DialogTrigger>
            ) : (
                <DialogTrigger asChild>
                    <Button
                        className="mt-0 rounded-sm w-full"
                        startContent={<SettingsIcon />}
                    >
                        {modelProvider.configured ? "Modify" : "Set Up"}
                    </Button>
                </DialogTrigger>
            )}

            <DialogDescription hidden>
                Configure Model Provider
            </DialogDescription>

            <DialogContent>
                <ModelProviderConfigureContent
                    modelProvider={modelProvider}
                    onSuccess={() => setDialogIsOpen(false)}
                />
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
    const { configureModelProvider } = useModelProviders();
    const revealModelProvider = useSWR(
        ModelProviderService.revealModelProviderById.key(modelProvider.id),
        ({ modelProviderId }) =>
            ModelProviderService.revealModelProviderById(modelProviderId)
    );

    const defaultRequiredConfigParams =
        modelProvider.requiredConfigurationParameters?.map(
            (requiredConfigParamKey) => ({
                name: requiredConfigParamKey,
                value: "",
            })
        ) ?? [];

    const form = useForm<ModelProviderFormValues>({
        resolver: zodResolver(formSchema),
        mode: "onChange",
        defaultValues: {
            requiredConfigParams: defaultRequiredConfigParams,
            additionalConfirmParams: [
                {
                    name: "",
                    value: "",
                },
            ],
        },
    });

    const requiredConfigParamFields = useFieldArray({
        control: form.control,
        name: "requiredConfigParams",
    });

    useEffect(() => {
        if (revealModelProvider.data) {
            const currentRequiredFieldValues = requiredConfigParamFields.fields;
            currentRequiredFieldValues.forEach((field, index) => {
                if (revealModelProvider.data?.[field.name]) {
                    form.setValue(
                        `requiredConfigParams.${index}.value`,
                        revealModelProvider.data[field.name]
                    );
                }
            });
        }
    }, [revealModelProvider.data, form, requiredConfigParamFields.fields]);

    const additionalConfirmParams = useFieldArray({
        control: form.control,
        name: "additionalConfirmParams",
    });

    const { execute: onSubmit, isLoading } = useAsync(
        async (data: ModelProviderFormValues) => {
            const allConfigParams: Record<string, string> = {};
            [data.requiredConfigParams, data.additionalConfirmParams].forEach(
                (configParams) => {
                    for (const param of configParams) {
                        if (param.name && param.value) {
                            allConfigParams[param.name] = param.value;
                        }
                    }
                }
            );

            configureModelProvider(modelProvider.id, allConfigParams);
            onSuccess();
        }
    );

    return (
        <>
            <DialogHeader>
                <DialogTitle className="mb-4 flex items-center gap-2">
                    <BoxesIcon />{" "}
                    {modelProvider.configured
                        ? `Configure ${modelProvider.name}`
                        : `Set Up ${modelProvider.name}`}
                </DialogTitle>

                <Form {...form}>
                    <form
                        onSubmit={form.handleSubmit(onSubmit)}
                        className="flex flex-col gap-8"
                    >
                        <div className="flex flex-col gap-4">
                            <fieldset className="flex flex-col gap-4">
                                <legend className="font-semibold mb-2">
                                    Required Configuration
                                </legend>
                                {requiredConfigParamFields.fields.map(
                                    (field, i) => (
                                        <ControlledInput
                                            key={field.id}
                                            label={field.name}
                                            control={form.control}
                                            name={`requiredConfigParams.${i}.value`}
                                            classNames={{
                                                wrapper:
                                                    "flex-auto bg-background",
                                            }}
                                        />
                                    )
                                )}
                            </fieldset>

                            <Separator className="my-4" />

                            <fieldset className="flex flex-col items-center gap-2">
                                <legend className="font-semibold mb-2">
                                    Custom Configuration (Optional)
                                </legend>
                                {additionalConfirmParams.fields.map(
                                    (field, i) => (
                                        <div
                                            className="flex gap-2 p-2 bg-secondary rounded-md w-full"
                                            key={field.id}
                                        >
                                            <ControlledInput
                                                control={form.control}
                                                name={`additionalConfirmParams.${i}.name`}
                                                placeholder="Name"
                                                classNames={{
                                                    wrapper:
                                                        "flex-auto bg-background",
                                                }}
                                            />

                                            <ControlledInput
                                                control={form.control}
                                                name={`additionalConfirmParams.${i}.value`}
                                                placeholder="Value"
                                                classNames={{
                                                    wrapper:
                                                        "flex-auto bg-background",
                                                }}
                                            />

                                            <Button
                                                variant="ghost"
                                                size="icon"
                                                onClick={() =>
                                                    additionalConfirmParams.remove(
                                                        i
                                                    )
                                                }
                                            >
                                                <TrashIcon />
                                            </Button>
                                        </div>
                                    )
                                )}
                                <Button
                                    variant="ghost"
                                    className="self-end"
                                    startContent={<PlusIcon />}
                                    onClick={() =>
                                        additionalConfirmParams.append({
                                            name: "",
                                            value: "",
                                        })
                                    }
                                >
                                    Add
                                </Button>
                            </fieldset>
                        </div>
                        <div className="flex justify-end">
                            <Button
                                type="submit"
                                disabled={isLoading}
                                loading={isLoading}
                            >
                                Confirm
                            </Button>
                        </div>
                    </form>
                </Form>
            </DialogHeader>
        </>
    );
}
