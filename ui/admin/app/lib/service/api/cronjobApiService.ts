import { z } from "zod";

import { CronJob, CronJobBase } from "~/lib/model/cronjobs";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";

const keys = {
	all: () => ["cronjobs"],
	byId: (id: string) => ["cronjobs", id],
};

const getAll = createFetcher(
	z.object({
		filters: z
			.object({
				workflowId: z.string().optional(),
			})
			.optional(),
	}),
	async function getCronJobs({ filters = {} }, { signal }) {
		const { workflowId } = filters;

		const { url } = ApiRoutes.cronjobs.getCronJobs();
		const { data } = await request<EntityList<CronJob>>({ url, signal });

		if (!workflowId) return data.items ?? [];

		return data.items?.filter((item) => item.workflow === workflowId) ?? [];
	},
	({ filters }) => [...keys.all(), filters]
);

const getById = createFetcher(
	z.object({ id: z.string() }),
	async ({ id }, { signal }) => {
		const { url } = ApiRoutes.cronjobs.getCronJobById(id);
		const { data } = await request<CronJob>({ url, signal });
		return data;
	},
	({ id }) => keys.byId(id)
);

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
	getCronJobs: getAll,
	getCronJobById: getById,
	createCronJob,
	deleteCronJob,
	updateCronJob,
};
