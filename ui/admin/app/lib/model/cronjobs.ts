import { EntityMeta } from "~/lib/model/primitives";

export type CronJobBase = {
	description: string;
	workflow: string;
	schedule: string; // cron string
	timezone: string;
	input?: string;
};

export type CronJob = EntityMeta & CronJobBase;
