import { zodResolver } from "@hookform/resolvers/zod";
import { CircleAlertIcon } from "lucide-react";
import { useEffect } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { mutate } from "swr";
import { z } from "zod";

import { ModelProvider, ModelProviderConfig } from "~/lib/model/modelProviders";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import {
	HelperTooltipLabel,
	HelperTooltipLink,
} from "~/components/composed/HelperTooltip";
import {
	NameDescriptionForm,
	ParamFormValues,
} from "~/components/composed/NameDescriptionForm";
import { ControlledInput } from "~/components/form/controlledInputs";
import {
	ModelProviderConfigurationLinks,
	ModelProviderRequiredTooltips,
	ModelProviderSensitiveFields,
} from "~/components/model-providers/constants";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
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
		"OBOT_OPENAI_MODEL_PROVIDER",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER",
		"OBOT_ANTHROPIC_MODEL_PROVIDER",
		"OBOT_OLLAMA_MODEL_PROVIDER",
		"OBOT_VOYAGE_MODEL_PROVIDER",
		"OBOT_GROQ_MODEL_PROVIDER",
		"OBOT_VLLM_MODEL_PROVIDER",
		"OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER",
		"OBOT_XAI_MODEL_PROVIDER",
		"OBOT_DEEPSEEK_MODEL_PROVIDER",
		"OBOT_GEMINI_VERTEX_MODEL_PROVIDER",
	];

	return fieldsToStrip
		.reduce((acc, field) => {
			return acc.replace(field, "");
		}, label)
		.toLowerCase()
		.replace(/_/g, " ")
		.replace(/\b\w/g, (char: string) => char.toUpperCase())
		.trim();
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
	onSuccess: () => void;
	parameters: ModelProviderConfig;
	requiredParameters: string[];
}) {
	const fetchAvailableModels = useAsync(
		ModelApiService.getAvailableModelsByProvider,
		{
			onSuccess: () => {
				mutate(ModelProviderApiService.getModelProviders.key());
				onSuccess();
			},
		}
	);

    const validateAndConfigureModelProvider = useAsync(
        ModelProviderApiService.validateModelProviderById,
        {
            onSuccess: async (data, params) => {
                // Only configure the model provider if validation was successful
                const [modelProviderId, configParams] = params;
                await configureModelProvider.execute(
                    modelProviderId,
                    configParams
                );
            },
            onError: (error) => {
                // Handle validation errors
                console.error("Validation failed:", error);
            },
        }
    );

    const configureModelProvider = useAsync(
        ModelProviderApiService.configureModelProviderById,
        {
            onSuccess: async () => {
                mutate(
                    ModelProviderApiService.revealModelProviderById.key(
                        modelProvider.id
                    )
                );
                await fetchAvailableModels.execute(modelProvider.id);
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

	useEffect(() => {
		form.reset({
			requiredConfigParams: getInitialRequiredParams(
				requiredParameters,
				parameters
			),
			additionalConfirmParams: getInitialAdditionalParams(
				requiredParameters,
				parameters
			),
		});
	}, [requiredParameters, parameters, form]);

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

            await validateAndConfigureModelProvider.execute(
                modelProvider.id,
                allConfigParams
            );
        }
    );

	const FORM_ID = "model-provider-form";
	const showCustomConfiguration =
		modelProvider.id === "azure-openai-model-provider";

    const loading =
        validateAndConfigureModelProvider.isLoading ||
        fetchAvailableModels.isLoading ||
        configureModelProvider.isLoading ||
        isLoading;

    return (
        <div className="flex flex-col">
            {validateAndConfigureModelProvider.error !== null && (
                <div className="px-4">
                    <Alert variant="destructive">
                        <CircleAlertIcon className="w-4 h-4" />
                        <AlertTitle>An error occurred!</AlertTitle>
                        <AlertDescription>
                            Your configuration could not be saved, because it
                            failed validation:{" "}
                            <strong>
                                {(typeof validateAndConfigureModelProvider.error ===
                                    "object" &&
                                    "message" in
                                        validateAndConfigureModelProvider.error &&
                                    (validateAndConfigureModelProvider.error
                                        .message as string)) ??
                                    "Unknown error"}
                            </strong>
                        </AlertDescription>
                    </Alert>
                </div>
            )}
            {validateAndConfigureModelProvider.error === null &&
                fetchAvailableModels.error !== null && (
                    <div className="px-4">
                        <Alert variant="destructive">
                            <CircleAlertIcon className="w-4 h-4" />
                            <AlertTitle>An error occurred!</AlertTitle>
                            <AlertDescription>
                                Your configuration was saved, but we were not
                                able to connect to the model provider. Please
                                check your configuration and try again:{" "}
                                <strong>
                                    {(typeof fetchAvailableModels.error ===
                                        "object" &&
                                        "message" in
                                            fetchAvailableModels.error &&
                                        (fetchAvailableModels.error
                                            .message as string)) ??
                                        "Unknown error"}
                                </strong>
                            </AlertDescription>
                        </Alert>
                    </div>
                )}
            <ScrollArea className="max-h-[50vh]">
                <div className="flex flex-col gap-4 p-4">
                    <h4 className="font-semibold text-md">
                        Required Configuration
                    </h4>
                    <Form {...form}>
                        <form
                            id={FORM_ID}
                            onSubmit={form.handleSubmit(onSubmit)}
                            className="flex flex-col gap-4"
                        >
                            {requiredConfigParamFields.fields.map(
                                (field, i) => {
                                    const type = ModelProviderSensitiveFields[
                                        field.name
                                    ]
                                        ? "password"
                                        : "text";

								return (
									<div
										key={field.id}
										className="flex items-center justify-center gap-2"
									>
										<ControlledInput
											key={field.id}
											label={renderLabelWithTooltip(field.label)}
											control={form.control}
											name={`requiredConfigParams.${i}.value`}
											type={type}
											classNames={{
												wrapper: "flex-auto bg-background",
											}}
										/>
									</div>
								);
							})}
						</form>
					</Form>

					{showCustomConfiguration && renderCustomConfiguration()}
				</div>
			</ScrollArea>

			<div className="flex justify-end px-6 py-4">
				<Button
					form={FORM_ID}
					disabled={loading}
					loading={loading}
					type="submit"
				>
					Confirm
				</Button>
			</div>
		</div>
	);

	function renderCustomConfiguration() {
		return (
			<>
				<Separator className="my-4" />

				<div className="flex items-center">
					<h4 className="text-md font-semibold">
						Custom Configuration (Optional)
					</h4>
					{ModelProviderConfigurationLinks[modelProvider.id]
						? renderCustomConfigTooltip(modelProvider.id)
						: null}
				</div>
				<NameDescriptionForm
					descriptionFieldProps={{ type: "password" }}
					defaultValues={form.watch("additionalConfirmParams")}
					onChange={(values) =>
						form.setValue("additionalConfirmParams", values)
					}
				/>
			</>
		);
	}

	function renderCustomConfigTooltip(modelProviderId: string) {
		const link = ModelProviderConfigurationLinks[modelProviderId];
		return <HelperTooltipLink link={link} />;
	}

	function renderLabelWithTooltip(label: string) {
		const tooltip = ModelProviderRequiredTooltips[modelProvider.id]?.[label];
		return <HelperTooltipLabel label={label} tooltip={tooltip} />;
	}
}
