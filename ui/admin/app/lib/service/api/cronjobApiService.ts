import { CronJob, CronJobBase } from "~/lib/model/cronjobs";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

type CronJobFilters = {
    workflowId?: string;
};

async function getCronJobs(filters?: CronJobFilters) {
    const { workflowId } = filters ?? {};

    const { data } = await request<{ items: CronJob[] }>({
        url: ApiRoutes.cronjobs.getCronJobs().url,
    });

    if (!workflowId) return data.items ?? [];

    return data.items?.filter((item) => item.workflow === workflowId) ?? [];
}
getCronJobs.key = (filters: CronJobFilters = {}) => ({
    url: ApiRoutes.cronjobs.getCronJobs().path,
    filters,
});
getCronJobs.revalidate = () =>
    revalidateWhere((url) => url === ApiRoutes.cronjobs.getCronJobs().path);

async function getCronJobById(cronJobId: string) {
    const res = await request<CronJob>({
        url: ApiRoutes.cronjobs.getCronJobById(cronJobId).url,
    });

    return res.data;
}
getCronJobById.key = (cronJobId: string) => ({
    url: ApiRoutes.cronjobs.getCronJobById(cronJobId).path,
    cronJobId,
});

async function createCronJob(cronJob: CronJobBase) {
    const res = await request<{ item: CronJob }>({
        url: ApiRoutes.cronjobs.createCronJob().url,
        method: "POST",
        data: cronJob,
        errorMessage: "Failed to create cronjob.",
    });

    return res.data;
}

async function deleteCronJob(cronJobId: string) {
    await request({
        url: ApiRoutes.cronjobs.deleteCronJob(cronJobId).url,
        method: "DELETE",
        errorMessage: "Failed to delete cronjob.",
    });
}

async function updateCronJob(cronJobId: string, cronJob: CronJobBase) {
    const res = await request<{ item: CronJob }>({
        url: ApiRoutes.cronjobs.updateCronJob(cronJobId).url,
        method: "PUT",
        data: cronJob,
        errorMessage: "Failed to update cronjob.",
    });

    return res.data;
}

export const CronJobApiService = {
    getCronJobs,
    getCronJobById,
    createCronJob,
    deleteCronJob,
    updateCronJob,
};
