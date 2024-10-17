import { OAuthApp, OAuthAppBase, OAuthAppSpec } from "~/lib/model/oauthApps";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

import { request } from "./primitives";

const getOauthApps = async () => {
    const res = await request<{ items: OAuthApp[] }>({
        url: ApiRoutes.oauthApps.getOauthApps().url,
        errorMessage: "Failed to get OAuth apps",
    });

    return res.data.items ?? ([] as OAuthApp[]);
};
getOauthApps.key = () =>
    ({ url: ApiRoutes.oauthApps.getOauthApps().path }) as const;

const getOauthAppById = async (id: string) => {
    const res = await request<OAuthApp>({
        url: ApiRoutes.oauthApps.getOauthAppById(id).url,
        errorMessage: "Failed to get OAuth app",
    });

    return res.data;
};
getOauthAppById.key = (id?: Nullish<string>) => {
    if (!id) return null;

    return { url: ApiRoutes.oauthApps.getOauthAppById(id).path, id };
};

const createOauthApp = async (oauthApp: OAuthAppBase) => {
    const res = await request<OAuthApp>({
        url: ApiRoutes.oauthApps.createOauthApp().url,
        method: "POST",
        data: oauthApp,
        errorMessage: "Failed to create OAuth app",
    });

    return res.data;
};

const updateOauthApp = async (id: string, oauthApp: OAuthAppBase) => {
    const res = await request<OAuthApp>({
        url: ApiRoutes.oauthApps.updateOauthApp(id).url,
        method: "PATCH",
        data: oauthApp,
        errorMessage: "Failed to update OAuth app",
    });

    return res.data;
};

const deleteOauthApp = async (id: string) => {
    await request({
        url: ApiRoutes.oauthApps.deleteOauthApp(id).url,
        method: "DELETE",
        errorMessage: "Failed to delete OAuth app",
    });
};

const getSupportedOauthAppTypes = async () => {
    const res = await request<OAuthAppSpec>({
        url: ApiRoutes.oauthApps.supportedOauthAppTypes().url,
        errorMessage: "Failed to get supported OAuth app types",
    });

    return new Map(Object.entries(res.data));
};
getSupportedOauthAppTypes.key = () =>
    ({ url: ApiRoutes.oauthApps.supportedOauthAppTypes().path }) as const;

const getSupportedAuthTypes = async () => {
    const res = await request({
        url: ApiRoutes.oauthApps.supportedAuthTypes().url,
        errorMessage: "Failed to get supported auth types",
    });

    return res.data;
};
getSupportedAuthTypes.key = () =>
    ({ url: ApiRoutes.oauthApps.supportedAuthTypes().path }) as const;

export const OauthAppService = {
    getOauthApps,
    getOauthAppById,
    createOauthApp,
    updateOauthApp,
    deleteOauthApp,
    getSupportedOauthAppTypes,
    getSupportedAuthTypes,
};
