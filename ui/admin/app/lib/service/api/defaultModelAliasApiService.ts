import {
    CreateDefaultModelAlias,
    DefaultModelAlias,
    UpdateDefaultModelAlias,
} from "~/lib/model/defaultModelAliases";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getAliases() {
    const { data } = await request<{ items: DefaultModelAlias[] }>({
        url: ApiRoutes.defaultModelAliases.getAliases().url,
    });

    return data.items;
}
getAliases.key = () => ({
    url: ApiRoutes.defaultModelAliases.getAliases().url,
});

async function getAliasById(aliasId: string) {
    const { data } = await request<DefaultModelAlias>({
        url: ApiRoutes.defaultModelAliases.getAliasById(aliasId).url,
    });

    return data;
}

async function createAlias(alias: CreateDefaultModelAlias) {
    const { data } = await request<DefaultModelAlias>({
        url: ApiRoutes.defaultModelAliases.createAlias().url,
        method: "POST",
        data: alias,
    });

    return data;
}

async function updateAlias(id: string, alias: UpdateDefaultModelAlias) {
    const { data } = await request<DefaultModelAlias>({
        url: ApiRoutes.defaultModelAliases.updateAlias(id).url,
        method: "PUT",
        data: alias,
    });

    return data;
}

async function deleteAlias(aliasId: string) {
    await request({
        url: ApiRoutes.defaultModelAliases.deleteAlias(aliasId).url,
        method: "DELETE",
    });
}

export const DefaultModelAliasApiService = {
    getAliases,
    getAliasById,
    createAlias,
    updateAlias,
    deleteAlias,
};
