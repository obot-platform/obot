import { ModelProvider, ModelProviderConfig } from "~/lib/model/modelProviders";
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

const getModelProviderById = async (modelProviderKey: string) => {
    const res = await request<ModelProvider>({
        url: ApiRoutes.modelProviders.getModelProviderById(modelProviderKey)
            .url,
        method: "GET",
        errorMessage:
            "Failed to update configuration values on the requested modal provider.",
    });

    return res.data;
};
getModelProviderById.key = (modelProviderId?: string) => {
    if (!modelProviderId) return null;

    return {
        url: ApiRoutes.modelProviders.getModelProviderById(modelProviderId)
            .path,
        modelProviderId,
    };
};

const configureModelProviderById = async (
    modelProviderKey: string,
    modelProviderConfig: ModelProviderConfig
) => {
    const res = await request<ModelProvider>({
        url: ApiRoutes.modelProviders.configureModelProviderById(
            modelProviderKey
        ).url,
        method: "POST",
        data: modelProviderConfig,
        errorMessage:
            "Failed to update configuration values on the requested modal provider.",
    });

    return res.data;
};
configureModelProviderById.key = (modelProviderId?: string) => {
    if (!modelProviderId) return null;

    return {
        url: ApiRoutes.modelProviders.configureModelProviderById(
            modelProviderId
        ).path,
        modelProviderId,
    };
};

const revealModelProviderById = async (modelProviderKey: string) => {
    const res = await request<ModelProviderConfig>({
        url: ApiRoutes.modelProviders.revealModelProviderById(modelProviderKey)
            .url,
        method: "POST",
        errorMessage:
            "Failed to reveal configuration values on the requested modal provider.",
    });

    return res.data;
};
revealModelProviderById.key = (modelProviderId?: string) => {
    if (!modelProviderId) return null;

    return {
        url: ApiRoutes.modelProviders.revealModelProviderById(modelProviderId)
            .path,
        modelProviderId,
    };
};

export const ModelProviderService = {
    getModelProviders,
    getModelProviderById,
    configureModelProviderById,
    revealModelProviderById,
};
