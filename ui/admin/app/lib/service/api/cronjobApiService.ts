import { CronJob, CronJobBase } from "~/lib/model/cronjobs";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getCronJobs() {
    const { data } = await request<{ items: CronJob[] }>({
        url: ApiRoutes.cronjobs.getCronJobs().url,
    });

    return data.items ?? [];
}
getCronJobs.key = () => ({ url: ApiRoutes.cronjobs.getCronJobs().path });

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

async function updateCronJob(cronJobId: string, cronJob: CronJob) {
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
    createCronJob,
    deleteCronJob,
    updateCronJob,
};
