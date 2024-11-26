import { Model, ModelManifest, ModelProvider } from "~/lib/model/models";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getModels() {
    const { data } = await request<{ items?: Model[] }>({
        url: ApiRoutes.models.getModels().url,
    });

    return data.items ?? [];
}
getModels.key = () => ({ url: ApiRoutes.models.getModels().path });

async function getModelById(modelId: string) {
    const { data } = await request<Model>({
        url: ApiRoutes.models.getModelById(modelId).url,
    });

    return data;
}
getModelById.key = (modelId?: string) => {
    if (!modelId) return null;

    return {
        url: ApiRoutes.models.getModelById(modelId).path,
        modelId,
    };
};

async function getModelProviders() {
    const { data } = await request<{ items?: ModelProvider[] }>({
        url: ApiRoutes.toolReferences.base({ type: "modelProvider" }).url,
    });

    return data.items ?? [];
}
getModelProviders.key = () => ({
    url: ApiRoutes.toolReferences.base({ type: "modelProvider" }).path,
});

async function createModel(manifest: ModelManifest) {
    await new Promise((resolve) => setTimeout(resolve, 1000));
    const { data } = await request<Model>({
        url: ApiRoutes.models.createModel().url,
        method: "POST",
        data: manifest,
    });

    return data;
}

async function updateModel(modelId: string, manifest: ModelManifest) {
    await new Promise((resolve) => setTimeout(resolve, 1000));

    const { data } = await request<Model>({
        url: ApiRoutes.models.updateModel(modelId).url,
        method: "PUT",
        data: manifest,
    });

    return data;
}

async function deleteModel(modelId: string) {
    await request({
        url: ApiRoutes.models.deleteModel(modelId).url,
        method: "DELETE",
    });
}

export const ModelApiService = {
    getModels,
    getModelById,
    getModelProviders,
    createModel,
    updateModel,
    deleteModel,
};
