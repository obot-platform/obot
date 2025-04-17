import { ModelProvider, ProviderConfig } from "~/lib/model/providers";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getModelProviders = async () => {
	const res = await request<{ items: ModelProvider[] }>({
		url: ApiRoutes.modelProviders.getModelProviders().url,
		errorMessage: "Failed to get supported model providers.",
	});

	return res.data.items ?? ([] as ModelProvider[]);
};
getModelProviders.key = () =>
	({ url: ApiRoutes.modelProviders.getModelProviders().path }) as const;

const getModelProviderById = async (providerKey: string) => {
	const res = await request<ModelProvider>({
		url: ApiRoutes.modelProviders.getModelProviderById(providerKey).url,
		method: "GET",
		errorMessage:
			"Failed to update configuration values on the requested model provider.",
	});

	return res.data;
};
getModelProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.modelProviders.getModelProviderById(providerId).path,
		providerId,
	};
};

const validateModelProviderById = async (
	modelProviderKey: string,
	modelProviderConfig: ProviderConfig
) => {
	const res = await request<ModelProvider>({
		url: ApiRoutes.modelProviders.validateModelProviderById(modelProviderKey)
			.url,
		method: "POST",
		data: modelProviderConfig,
		errorMessage:
			"Failed to validate configuration values on the requested model provider.",
	});

	return res.data;
};

const configureModelProviderById = async (
	providerKey: string,
	providerConfig: ProviderConfig
) => {
	const res = await request<ModelProvider>({
		url: ApiRoutes.modelProviders.configureModelProviderById(providerKey).url,
		method: "POST",
		data: providerConfig,
		errorMessage:
			"Failed to update configuration values on the requested model provider.",
	});

	return res.data;
};

const revealModelProviderById = async (providerKey: string) => {
	const res = await request<ProviderConfig>({
		url: ApiRoutes.modelProviders.revealModelProviderById(providerKey).url,
		method: "POST",
		errorMessage:
			"Failed to reveal configuration values on the requested model provider.",
		toastError: false,
	});

	return res.data;
};
revealModelProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.modelProviders.revealModelProviderById(providerId).path,
		providerId,
	};
};

const deconfigureModelProviderById = async (providerKey: string) => {
	const res = await request<ModelProvider>({
		url: ApiRoutes.modelProviders.deconfigureModelProviderById(providerKey).url,
		method: "POST",
		errorMessage: "Failed to deconfigure the requested model provider.",
	});

	return res.data;
};

export const ModelProviderApiService = {
	getModelProviders,
	getModelProviderById,
	validateModelProviderById,
	configureModelProviderById,
	revealModelProviderById,
	deconfigureModelProviderById,
};
