import { zodResolver } from "@hookform/resolvers/zod";
import { CircleAlertIcon } from "lucide-react";
import { useEffect } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { mutate } from "swr";
import { z } from "zod";

import {
	AuthProvider,
	FileScannerProvider,
	ModelProvider,
	ProviderConfig,
	ProviderConfigurationParameter,
} from "~/lib/model/providers";
import { AuthProviderApiService } from "~/lib/service/api/authProviderApiService";
import { FileScannerProviderApiService } from "~/lib/service/api/fileScannerProviderApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { HelperTooltipLabel } from "~/components/composed/HelperTooltip";
import { ControlledInput } from "~/components/form/controlledInputs";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useAsync } from "~/hooks/useAsync";

const formSchema = z.object({
	requiredConfigParams: z.array(
		z.object({
			label: z.string(),
			tooltip: z.string(),
			name: z.string().min(1, {
				message: "Name is required.",
			}),
			value: z.string().min(1, {
				message: "This field is required.",
			}),
			sensitive: z.boolean(),
		})
	),
	optionalConfigParams: z.array(
		z.object({
			label: z.string(),
			tooltip: z.string(),
			name: z.string().min(1, {
				message: "Name is required.",
			}),
			value: z.string(),
			sensitive: z.boolean(),
		})
	),
});

export type ProviderFormValues = z.infer<typeof formSchema>;

const getInitialRequiredParams = (
	requiredParameters: ProviderConfigurationParameter[],
	parameters: ProviderConfig
): ProviderFormValues["requiredConfigParams"] =>
	requiredParameters
		.filter((param) => !param.hidden)
		.map((param) => ({
			tooltip: param.description ?? "",
			label: param.friendlyName ?? param.name,
			name: param.name,
			value: parameters[param.name] ?? "",
			sensitive: param.sensitive ?? false,
		}));

const getInitialOptionalParams = (
	optionalParameters: ProviderConfigurationParameter[],
	parameters: ProviderConfig
): ProviderFormValues["optionalConfigParams"] =>
	optionalParameters
		.filter((param) => !param.hidden)
		.map((param) => ({
			tooltip: param.description ?? "",
			label: param.friendlyName ?? param.name,
			name: param.name,
			value: parameters[param.name] ?? "",
			sensitive: param.sensitive ?? false,
			hidden: param.hidden ?? false,
		}));

export function ProviderForm({
	provider,
	onSuccess,
	parameters,
	requiredParameters,
	optionalParameters,
}: {
	provider: ModelProvider | AuthProvider | FileScannerProvider;
	onSuccess: () => void;
	parameters: ProviderConfig;
	requiredParameters: ProviderConfigurationParameter[];
	optionalParameters: ProviderConfigurationParameter[];
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

	const configureAuthProvider = useAsync(
		AuthProviderApiService.configureAuthProviderById,
		{
			onSuccess: async () => {
				onSuccess();
				mutate(AuthProviderApiService.getAuthProviders.key());
				mutate(AuthProviderApiService.revealAuthProviderById.key(provider.id));
			},
		}
	);

	const validateAndConfigureModelProvider = useAsync(
		ModelProviderApiService.validateModelProviderById,
		{
			onSuccess: async (data, params) => {
				// Only configure the model provider if validation was successful
				const [modelProviderId, configParams] = params;
				await configureModelProvider.execute(modelProviderId, configParams);
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
					ModelProviderApiService.revealModelProviderById.key(provider.id)
				);
				await fetchAvailableModels.execute(provider.id);
			},
		}
	);

	const validateAndConfigureFileScannerProvider = useAsync(
		FileScannerProviderApiService.validateFileScannerProviderById,
		{
			onSuccess: async (data, params) => {
				// Only configure the model provider if validation was successful
				const [fileScannerProviderId, configParams] = params;
				await configureFileScannerProvider.execute(
					fileScannerProviderId,
					configParams
				);
			},
			onError: (error) => {
				// Handle validation errors
				console.error("Validation failed:", error);
			},
		}
	);

	const configureFileScannerProvider = useAsync(
		FileScannerProviderApiService.configureFileScannerProviderById,
		{
			onSuccess: async () => {
				onSuccess();
				mutate(FileScannerProviderApiService.getFileScannerProviders.key());
				mutate(
					FileScannerProviderApiService.revealFileScannerProviderById.key(
						provider.id
					)
				);
			},
		}
	);

	const form = useForm<ProviderFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues: {
			requiredConfigParams: getInitialRequiredParams(
				requiredParameters,
				parameters
			),
			optionalConfigParams: getInitialOptionalParams(
				optionalParameters,
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
			optionalConfigParams: getInitialOptionalParams(
				optionalParameters,
				parameters
			),
		});
	}, [requiredParameters, optionalParameters, parameters, form]);

	const requiredConfigParamFields = useFieldArray({
		control: form.control,
		name: "requiredConfigParams",
	});

	const optionalConfigParamFields = useFieldArray({
		control: form.control,
		name: "optionalConfigParams",
	});

	const { execute: onSubmit, isLoading } = useAsync(
		async (data: ProviderFormValues) => {
			const allConfigParams: Record<string, string> = {};
			[data.requiredConfigParams, data.optionalConfigParams].forEach(
				(configParams) => {
					for (const param of configParams) {
						if (param.value && param.name) {
							allConfigParams[param.name] = param.value;
						}
					}
				}
			);

			switch (provider.type) {
				case "modelprovider":
					await validateAndConfigureModelProvider.execute(
						provider.id,
						allConfigParams
					);
					break;
				case "authprovider":
					await configureAuthProvider.execute(provider.id, allConfigParams);
					break;
				case "filescannerprovider":
					await validateAndConfigureFileScannerProvider.execute(
						provider.id,
						allConfigParams
					);
					break;
			}
		}
	);

	const FORM_ID = "model-provider-form";

	const loading =
		validateAndConfigureModelProvider.isLoading ||
		fetchAvailableModels.isLoading ||
		configureModelProvider.isLoading ||
		configureAuthProvider.isLoading ||
		isLoading;

	return (
		<div className="flex flex-col">
			{provider.type === "modelprovider" &&
				validateAndConfigureModelProvider.error !== null && (
					<div className="px-4">
						<Alert variant="destructive">
							<CircleAlertIcon className="h-4 w-4" />
							<AlertTitle>An error occurred!</AlertTitle>
							<AlertDescription>
								Your configuration could not be saved, because it failed
								validation:{" "}
								<strong>
									{(typeof validateAndConfigureModelProvider.error ===
										"object" &&
										"message" in validateAndConfigureModelProvider.error &&
										(validateAndConfigureModelProvider.error
											.message as string)) ??
										"Unknown error"}
								</strong>
							</AlertDescription>
						</Alert>
					</div>
				)}
			{provider.type === "filescannerprovider" &&
				validateAndConfigureFileScannerProvider.error !== null && (
					<div className="px-4">
						<Alert variant="destructive">
							<CircleAlertIcon className="h-4 w-4" />
							<AlertTitle>An error occurred!</AlertTitle>
							<AlertDescription>
								Your configuration could not be saved, because it failed
								validation:{" "}
								<strong>
									{(typeof validateAndConfigureFileScannerProvider.error ===
										"object" &&
										"message" in
											validateAndConfigureFileScannerProvider.error &&
										(validateAndConfigureFileScannerProvider.error
											.message as string)) ??
										"Unknown error"}
								</strong>
							</AlertDescription>
						</Alert>
					</div>
				)}
			{provider.type === "modelprovider" &&
				validateAndConfigureModelProvider.error === null &&
				fetchAvailableModels.error !== null && (
					<div className="px-4">
						<Alert variant="destructive">
							<CircleAlertIcon className="h-4 w-4" />
							<AlertTitle>An error occurred!</AlertTitle>
							<AlertDescription>
								Your configuration was saved, but we were not able to connect to
								the model provider. Please check your configuration and try
								again:{" "}
								<strong>
									{(typeof fetchAvailableModels.error === "object" &&
										"message" in fetchAvailableModels.error &&
										(fetchAvailableModels.error.message as string)) ??
										"Unknown error"}
								</strong>
							</AlertDescription>
						</Alert>
					</div>
				)}
			<ScrollArea className="max-h-[50vh]">
				<div className="flex flex-col gap-4 p-4">
					<Form {...form}>
						<form
							id={FORM_ID}
							onSubmit={form.handleSubmit(onSubmit)}
							className="flex flex-col gap-4"
						>
							<h4 className="text-md font-semibold">Required Configuration</h4>
							{requiredConfigParamFields.fields.map((field, i) => {
								const type = field.sensitive ? "password" : "text";

								return (
									<div
										key={field.id}
										className="flex items-center justify-center gap-2"
									>
										<ControlledInput
											key={field.id}
											label={renderLabelWithTooltip(field.label, field.tooltip)}
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
							{optionalParameters.length > 0 && (
								<h4 className="text-md font-semibold">
									Optional Configuration
								</h4>
							)}
							{optionalConfigParamFields.fields.map((field, i) => {
								const type = field.sensitive ? "password" : "text";

								return (
									<div
										key={field.id}
										className="flex items-center justify-center gap-2"
									>
										<ControlledInput
											key={field.id}
											label={renderLabelWithTooltip(field.label, field.tooltip)}
											control={form.control}
											name={`optionalConfigParams.${i}.value`}
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

	function renderLabelWithTooltip(label: string, tooltip: string) {
		return <HelperTooltipLabel label={label} tooltip={tooltip} />;
	}
}
