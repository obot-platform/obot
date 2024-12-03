import { ModelProvider } from "~/lib/model/models";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getModelProviders() {
    const { data } = await request<{ items?: ModelProvider[] }>({
        url: ApiRoutes.modelProviders.getModelProviders().url,
    });

    return data.items ?? [];
}
getModelProviders.key = () => ({
    url: ApiRoutes.modelProviders.getModelProviders().path,
});

export const ModelProviderApiService = { getModelProviders };
