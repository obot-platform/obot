import { zodResolver } from "@hookform/resolvers/zod";
import { CircleHelpIcon } from "lucide-react";
import { useFieldArray, useForm } from "react-hook-form";
import { toast } from "sonner";
import { mutate } from "swr";
import { z } from "zod";

import { ModelProvider, ModelProviderConfig } from "~/lib/model/modelProviders";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { TypographyH4 } from "~/components/Typography";
import {
    NameDescriptionForm,
    ParamFormValues,
} from "~/components/composed/NameDescriptionForm";
import { ControlledInput } from "~/components/form/controlledInputs";
import { ModelProviderConfigurationLinks } from "~/components/model-providers/constants";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { Link } from "~/components/ui/link";
import { ScrollArea } from "~/components/ui/scroll-area";
import { Separator } from "~/components/ui/separator";
import { useAsync } from "~/hooks/useAsync";

const formSchema = z.object({
    requiredConfigParams: z.array(
        z.object({
            label: z.string(),
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
            description: z.string(),
        })
    ),
});

export type ModelProviderFormValues = z.infer<typeof formSchema>;

const translateUserFriendlyLabel = (label: string) => {
    const fieldsToStrip = [
        "OTTO8_OPENAI_MODEL_PROVIDER",
        "OTTO8_AZURE_OPENAI_MODEL_PROVIDER",
        "OTTO8_ANTHROPIC_MODEL_PROVIDER",
        "OTTO8_OLLAMA_MODEL_PROVIDER",
        "OTTO8_VOYAGE_MODEL_PROVIDER",
    ];

    return fieldsToStrip
        .reduce((acc, field) => {
            return acc.replace(field, "");
        }, label)
        .toLowerCase()
        .replace(/_/g, " ")
        .replace(/\b\w/g, (char: string) => char.toUpperCase());
};

const getInitialRequiredParams = (
    requiredParameters: string[],
    parameters: ModelProviderConfig
): ModelProviderFormValues["requiredConfigParams"] =>
    requiredParameters.map((requiredParameterKey) => ({
        label: translateUserFriendlyLabel(requiredParameterKey),
        name: requiredParameterKey,
        value: parameters[requiredParameterKey] ?? "",
    }));

const getInitialAdditionalParams = (
    requiredParameters: string[],
    parameters: ModelProviderConfig
): ParamFormValues["params"] => {
    const defaultEmptyParams = [{ name: "", description: "" }];

    const requiredParameterSet = new Set(requiredParameters);
    const additionalParams = Object.entries(parameters).filter(
        ([key]) => !requiredParameterSet.has(key)
    );
    return additionalParams.length === 0
        ? defaultEmptyParams
        : additionalParams.map(([key, value]) => ({
              name: key,
              description: value,
          }));
};

export function ModelProviderForm({
    modelProvider,
    onSuccess,
    parameters,
    requiredParameters,
}: {
    modelProvider: ModelProvider;
    onSuccess: (config: ModelProviderConfig) => void;
    parameters: ModelProviderConfig;
    requiredParameters: string[];
}) {
    const configureModelProvider = useAsync(
        ModelProviderApiService.configureModelProviderById,
        {
            onSuccess: () => {
                mutate(ModelProviderApiService.getModelProviders.key());
                toast.success(`${modelProvider.name} configured successfully.`);
            },
        }
    );

    const form = useForm<ModelProviderFormValues>({
        resolver: zodResolver(formSchema),
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

    const requiredConfigParamFields = useFieldArray({
        control: form.control,
        name: "requiredConfigParams",
    });

    const { execute: onSubmit, isLoading } = useAsync(
        async (data: ModelProviderFormValues) => {
            const allConfigParams: Record<string, string> = {};
            [data.requiredConfigParams, data.additionalConfirmParams].forEach(
                (configParams) => {
                    for (const param of configParams) {
                        const paramValue =
                            "value" in param ? param.value : param.description;
                        if (paramValue && param.name) {
                            allConfigParams[param.name] = paramValue;
                        }
                    }
                }
            );

            await configureModelProvider.execute(
                modelProvider.id,
                allConfigParams
            );
            onSuccess(allConfigParams);
        }
    );

    const FORM_ID = "model-provider-form";
    const showCustomConfiguration =
        modelProvider.id === "azure-openai-model-provider";
    return (
        <div className="flex flex-col">
            <ScrollArea className="max-h-[50vh]">
                <div className="flex flex-col gap-4 p-4">
                    <TypographyH4 className="font-semibold text-md">
                        Required Configuration
                    </TypographyH4>
                    <Form {...form}>
                        <form
                            id={FORM_ID}
                            onSubmit={form.handleSubmit(onSubmit)}
                            className="flex flex-col gap-4"
                        >
                            {requiredConfigParamFields.fields.map(
                                (field, i) => (
                                    <ControlledInput
                                        key={field.id}
                                        label={field.label}
                                        control={form.control}
                                        name={`requiredConfigParams.${i}.value`}
                                        classNames={{
                                            wrapper: "flex-auto bg-background",
                                        }}
                                    />
                                )
                            )}
                        </form>
                    </Form>

                    {showCustomConfiguration ? (
                        <>
                            <Separator className="my-4" />

                            <div className="flex items-center gap-2">
                                <TypographyH4 className="font-semibold text-md">
                                    Custom Configuration (Optional)
                                </TypographyH4>
                                {ModelProviderConfigurationLinks[
                                    modelProvider.id
                                ] ? (
                                    <Link
                                        as="button"
                                        variant="ghost"
                                        size="icon"
                                        to={
                                            ModelProviderConfigurationLinks[
                                                modelProvider.id
                                            ]
                                        }
                                    >
                                        <CircleHelpIcon className="text-muted-foreground" />
                                    </Link>
                                ) : null}
                            </div>
                            <NameDescriptionForm
                                defaultValues={form.watch(
                                    "additionalConfirmParams"
                                )}
                                onChange={(values) =>
                                    form.setValue(
                                        "additionalConfirmParams",
                                        values
                                    )
                                }
                            />
                        </>
                    ) : null}
                </div>
            </ScrollArea>

            <div className="flex justify-end px-6 py-4">
                <Button
                    form={FORM_ID}
                    disabled={isLoading}
                    loading={isLoading}
                    type="submit"
                >
                    Confirm
                </Button>
            </div>
        </div>
    );
}
