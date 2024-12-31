import { Version } from "~/lib/model/version";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getVersion() {
    const res = await request<Version>({
        url: ApiRoutes.version().url,
        errorMessage: "Failed to fetch app version.",
    });

    return res.data;
}
getVersion.key = () => ({ url: ApiRoutes.version().path }) as const;

export const VersionApiService = {
    getVersion,
};
