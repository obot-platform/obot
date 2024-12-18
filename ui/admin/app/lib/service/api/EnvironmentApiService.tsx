import { RevealedEnv } from "~/lib/model/environmentVariables";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getEnvVariables(workflowId: string) {
    const res = await request<RevealedEnv>({
        url: ApiRoutes.env.getEnv(workflowId).url,
        errorMessage: "Failed to fetch workflow env",
    });

    return res.data;
}

async function updateEnvVariables(workflowId: string, env: RevealedEnv) {
    await request({
        url: ApiRoutes.env.updateEnv(workflowId).url,
        method: "POST",
        data: env,
        errorMessage: "Failed to update workflow env",
    });
}

export const EnvironmentApiService = {
    getEnvVariables,
    updateEnvVariables,
};
