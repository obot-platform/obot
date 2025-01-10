import { AvailableModel } from "~/lib/model/availableModels";
import { Model, ModelManifest } from "~/lib/model/models";
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

async function getAvailableModelsByProvider(provider: string) {
	const { data } = await request<{ data?: AvailableModel[] }>({
		url: ApiRoutes.models.getAvailableModelsByProvider(provider).url,
	});

	return data.data ?? [];
}
getAvailableModelsByProvider.key = (provider?: Nullish<string>) => {
	if (!provider) return null;

	return {
		url: ApiRoutes.models.getAvailableModelsByProvider(provider).path,
		provider,
	};
};

async function createModel(manifest: ModelManifest) {
	const { data } = await request<Model>({
		url: ApiRoutes.models.createModel().url,
		method: "POST",
		data: manifest,
	});

	return data;
}

async function updateModel(modelId: string, manifest: ModelManifest) {
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
	getAvailableModelsByProvider,
	createModel,
	updateModel,
	deleteModel,
};
