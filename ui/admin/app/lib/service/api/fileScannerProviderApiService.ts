import { FileScannerProvider, ProviderConfig } from "~/lib/model/providers";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getFileScannerProviders = async () => {
	const res = await request<{ items: FileScannerProvider[] }>({
		url: ApiRoutes.fileScannerProviders.getFileScannerProviders().url,
		errorMessage: "Failed to get supported file scanner providers.",
	});

	return res.data.items ?? ([] as FileScannerProvider[]);
};
getFileScannerProviders.key = () =>
	({
		url: ApiRoutes.fileScannerProviders.getFileScannerProviders().path,
	}) as const;

const getFileScannerProviderById = async (providerKey: string) => {
	const res = await request<FileScannerProvider>({
		url: ApiRoutes.fileScannerProviders.getFileScannerProviderById(providerKey)
			.url,
		method: "GET",
		errorMessage:
			"Failed to update configuration values on the requested file scanner provider.",
	});

	return res.data;
};
getFileScannerProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.fileScannerProviders.getFileScannerProviderById(providerId)
			.path,
		providerId,
	};
};

const validateFileScannerProviderById = async (
	fileScannerProviderKey: string,
	fileScannerProviderConfig: ProviderConfig
) => {
	const res = await request<FileScannerProvider>({
		url: ApiRoutes.fileScannerProviders.validateFileScannerProviderById(
			fileScannerProviderKey
		).url,
		method: "POST",
		data: fileScannerProviderConfig,
		errorMessage:
			"Failed to validate configuration values on the requested file scanner provider.",
	});

	return res.data;
};

const configureFileScannerProviderById = async (
	providerKey: string,
	providerConfig: ProviderConfig
) => {
	const res = await request<FileScannerProvider>({
		url: ApiRoutes.fileScannerProviders.configureFileScannerProviderById(
			providerKey
		).url,
		method: "POST",
		data: providerConfig,
		errorMessage:
			"Failed to update configuration values on the requested file scanner provider.",
	});

	return res.data;
};

const revealFileScannerProviderById = async (providerKey: string) => {
	const res = await request<ProviderConfig>({
		url: ApiRoutes.fileScannerProviders.revealFileScannerProviderById(
			providerKey
		).url,
		method: "POST",
		errorMessage:
			"Failed to reveal configuration values on the requested file scanner provider.",
		toastError: false,
	});

	return res.data;
};
revealFileScannerProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.fileScannerProviders.revealFileScannerProviderById(
			providerId
		).path,
		providerId,
	};
};

const deconfigureFileScannerProviderById = async (providerKey: string) => {
	const res = await request<FileScannerProvider>({
		url: ApiRoutes.fileScannerProviders.deconfigureFileScannerProviderById(
			providerKey
		).url,
		method: "POST",
		errorMessage: "Failed to deconfigure the requested file scanner provider.",
	});

	return res.data;
};

export const FileScannerProviderApiService = {
	getFileScannerProviders,
	getFileScannerProviderById,
	validateFileScannerProviderById,
	configureFileScannerProviderById,
	revealFileScannerProviderById,
	deconfigureFileScannerProviderById,
};
