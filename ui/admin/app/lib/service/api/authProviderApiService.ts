import {
	AuthProvider,
	ModelProvider,
	ProviderConfig,
} from "~/lib/model/providers";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getAuthProviders = async () => {
	const res = await request<{ items: AuthProvider[] }>({
		url: ApiRoutes.authProviders.getAuthProviders().url,
		errorMessage: "Failed to get supported auth providers.",
	});

	return res.data.items ?? ([] as AuthProvider[]);
};
getAuthProviders.key = () =>
	({ url: ApiRoutes.authProviders.getAuthProviders().path }) as const;

const getAuthProviderById = async (providerKey: string) => {
	const res = await request<AuthProvider>({
		url: ApiRoutes.authProviders.getAuthProviderById(providerKey).url,
		method: "GET",
		errorMessage:
			"Failed to update configuration values on the requested auth provider.",
	});

	return res.data;
};
getAuthProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.authProviders.getAuthProviderById(providerId).path,
		providerId,
	};
};

const configureAuthProviderById = async (
	providerKey: string,
	providerConfig: ProviderConfig
) => {
	const res = await request<AuthProvider>({
		url: ApiRoutes.authProviders.configureAuthProviderById(providerKey).url,
		method: "POST",
		data: providerConfig,
		errorMessage:
			"Failed to update configuration values on the requested auth provider.",
	});

	return res.data;
};

const revealAuthProviderById = async (providerKey: string) => {
	const res = await request<ProviderConfig>({
		url: ApiRoutes.authProviders.revealAuthProviderById(providerKey).url,
		method: "POST",
		errorMessage:
			"Failed to reveal configuration values on the requested auth provider.",
	});

	return res.data;
};
revealAuthProviderById.key = (providerId?: string) => {
	if (!providerId) return null;

	return {
		url: ApiRoutes.authProviders.revealAuthProviderById(providerId).path,
		providerId,
	};
};

const deconfigureAuthProviderById = async (providerKey: string) => {
	const res = await request<ModelProvider>({
		url: ApiRoutes.authProviders.deconfigureAuthProviderById(providerKey).url,
		method: "POST",
		errorMessage: "Failed to deconfigure the requested auth provider.",
	});

	return res.data;
};

export const AuthProviderApiService = {
	getAuthProviders,
	getAuthProviderById,
	configureAuthProviderById,
	revealAuthProviderById,
	deconfigureAuthProviderById,
};
