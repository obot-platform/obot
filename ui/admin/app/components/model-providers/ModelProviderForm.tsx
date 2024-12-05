import { zodResolver } from "@hookform/resolvers/zod";
import { useFieldArray, useForm } from "react-hook-form";
import { mutate } from "swr";
import { z } from "zod";

import { ModelProviderConfig } from "~/lib/model/modelProviders";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { TypographyH4 } from "~/components/Typography";
import {
    NameDescriptionForm,
    ParamFormValues,
} from "~/components/composed/NameDescriptionForm";
import { ControlledInput } from "~/components/form/controlledInputs";
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
            description: z.string(),
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
    modelProviderId,
    onSuccess,
    parameters,
    requiredParameters,
}: {
    modelProviderId: string;
    onSuccess: (config: ModelProviderConfig) => void;
    parameters: ModelProviderConfig;
    requiredParameters: string[];
}) {
    const configureModelProvider = useAsync(
        ModelProviderApiService.configureModelProviderById,
        {
            onSuccess: () =>
                mutate(ModelProviderApiService.getModelProviders.key()),
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
                modelProviderId,
                allConfigParams
            );
            onSuccess(allConfigParams);
        }
    );

    const FORM_ID = "model-provider-form";
    return (
        <div className="flex flex-col gap-4">
            <TypographyH4 className="font-semibold text-md">
                Required Configuration
            </TypographyH4>
            <Form {...form}>
                <form
                    id={FORM_ID}
                    onSubmit={form.handleSubmit(onSubmit)}
                    className="flex flex-col gap-8"
                >
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
                </form>
            </Form>

            <Separator className="my-4" />

            <TypographyH4 className="font-semibold text-md">
                Custom Configuration (Optional)
            </TypographyH4>
            <NameDescriptionForm
                defaultValues={form.watch("additionalConfirmParams")}
                onChange={(values) =>
                    form.setValue("additionalConfirmParams", values)
                }
            />

            <div className="flex justify-end">
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
