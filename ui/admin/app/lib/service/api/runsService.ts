import { Run, RunDebug } from "~/lib/model/runs";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

const getRuns = async () => {
    const res = await request<{ items: Run[] }>({
        url: ApiRoutes.runs.base().url,
        errorMessage: "Failed to fetch runs",
    });

    return res.data.items ?? ([] as Run[]);
};
getRuns.key = () => ({ url: ApiRoutes.runs.base().path });

const getRunDebugById = async (runId: string) => {
    const res = await request<RunDebug>({
        url: ApiRoutes.runs.getDebugById(runId).url,
        errorMessage: "Failed to fetch run debug",
    });

    return res.data;
};
getRunDebugById.key = (runId?: Nullish<string>) => {
    if (!runId) return null;

    return { url: ApiRoutes.runs.getDebugById(runId).path, runId };
};

const getRunById = async (runId: string) => {
    const res = await request<Run>({
        url: ApiRoutes.runs.getRunById(runId).url,
        errorMessage: "Failed to fetch run",
    });

    return res.data;
};

const getRunsByThread = async (threadId: string) => {
    const res = await request<{ items: Run[] }>({
        url: ApiRoutes.runs.getByThread(threadId).url,
        errorMessage: "Failed to fetch runs by thread",
    });

    return res.data.items;
};
getRunsByThread.key = (threadId?: Nullish<string>) => {
    if (!threadId) return null;

    return { url: ApiRoutes.runs.getByThread(threadId).path, threadId };
};

const revalidateRuns = () =>
    revalidateWhere((url) => url.includes(ApiRoutes.runs.base().path));

export const RunsService = {
    getRuns,
    getRunsByThread,
    revalidateRuns,
    getRunDebugById,
    getRunById,
};
