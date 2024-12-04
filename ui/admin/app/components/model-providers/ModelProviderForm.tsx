import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useEffect } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { ModelProviderConfig } from "~/lib/model/modelProviders";

import { ControlledInput } from "~/components/form/controlledInputs";
import { useModelProviders } from "~/components/model-providers/ModelProviderContext";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { Separator } from "~/components/ui/separator";
import { useAsync } from "~/hooks/useAsync";

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

const getInitialRequiredParams = (
    requiredParameters: string[],
    parameters: ModelProviderConfig
): ModelProviderFormValues["requiredConfigParams"] =>
    requiredParameters.map((requiredParameterKey) => ({
        name: requiredParameterKey,
        value: parameters[requiredParameterKey] ?? "",
    }));

const getInitialAdditionalParams = (
    requiredParameters: string[],
    parameters: ModelProviderConfig
): ModelProviderFormValues["additionalConfirmParams"] => {
    const defaultEmptyParams = [{ name: "", value: "" }];

    const requiredParameterSet = new Set(requiredParameters);
    const parameterKeys = Object.entries(parameters);
    return parameterKeys.length === 0
        ? defaultEmptyParams
        : parameterKeys
              .filter(([key, value]) => !requiredParameterSet.has(key))
              .map(([key, value]) => ({
                  name: key,
                  value,
              }));
};

export function ModelProviderForm({
    modelProviderId,
    onSuccess,
    parameters,
    requiredParameters,
}: {
    modelProviderId: string;
    onSuccess: () => void;
    parameters: ModelProviderConfig;
    requiredParameters: string[];
}) {
    const { configureModelProvider } = useModelProviders();

    const form = useForm<ModelProviderFormValues>({
        resolver: zodResolver(formSchema),
        mode: "onChange",
        defaultValues: {
            requiredConfigParams: getInitialRequiredParams(
                requiredParameters,
                parameters
            ),
            additionalConfirmParams: getInitialAdditionalParams(
                requiredParameters,
                parameters
            ),
        },
    });

    useEffect(() => {
        const updatedRequiredParams = getInitialRequiredParams(
            requiredParameters,
            parameters
        );
        form.setValue("requiredConfigParams", updatedRequiredParams);

        const updatedAdditionalParams = getInitialAdditionalParams(
            requiredParameters,
            parameters
        );
        form.setValue("additionalConfirmParams", updatedAdditionalParams);
    }, [parameters, requiredParameters]);

    const requiredConfigParamFields = useFieldArray({
        control: form.control,
        name: "requiredConfigParams",
    });

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

            configureModelProvider(modelProviderId, allConfigParams);
            console.log("A");
            onSuccess();
        }
    );

    return (
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
                        {requiredConfigParamFields.fields.map((field, i) => (
                            <ControlledInput
                                key={field.id}
                                label={field.name}
                                control={form.control}
                                name={`requiredConfigParams.${i}.value`}
                                classNames={{
                                    wrapper: "flex-auto bg-background",
                                }}
                            />
                        ))}
                    </fieldset>

                    <Separator className="my-4" />

                    <fieldset className="flex flex-col items-center gap-2">
                        <legend className="font-semibold mb-2">
                            Custom Configuration (Optional)
                        </legend>
                        {additionalConfirmParams.fields.map((field, i) => (
                            <div
                                className="flex gap-2 p-2 bg-secondary rounded-md w-full"
                                key={field.id}
                            >
                                <ControlledInput
                                    control={form.control}
                                    name={`additionalConfirmParams.${i}.name`}
                                    placeholder="Name"
                                    classNames={{
                                        wrapper: "flex-auto bg-background",
                                    }}
                                />

                                <ControlledInput
                                    control={form.control}
                                    name={`additionalConfirmParams.${i}.value`}
                                    placeholder="Value"
                                    classNames={{
                                        wrapper: "flex-auto bg-background",
                                    }}
                                />

                                <Button
                                    variant="ghost"
                                    size="icon"
                                    onClick={() =>
                                        additionalConfirmParams.remove(i)
                                    }
                                >
                                    <TrashIcon />
                                </Button>
                            </div>
                        ))}
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
    );
}
