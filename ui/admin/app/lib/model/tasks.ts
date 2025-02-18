import { EntityMeta } from "~/lib/model/primitives";

export type TaskBase = {
	alias: string;
	description: string;
	email: object | null;
	name: string;
	onDemand: {
		params?: Record<string, string>;
	} | null;
	schedule: {
		interval: string;
		hour: number;
		minute: number;
		day: number;
		weekday: number;
	} | null;
	steps: Step[] | null;
	threadID: string;
	webhook: object | null;
};

export type UpdateTask = TaskBase & {
	params?: Record<string, string> | null;
};
export type Task = EntityMeta & TaskBase;

export type Step = {
	id: string;
	step?: string;
};

export const getDefaultStep = (id?: string): Step => ({
	id: id || crypto.randomUUID(),
	step: "",
});

export type TaskRun = EntityMeta & {
	taskID: string;
	input: string;
	task: Task;
	startTime: Date;
	endTime: Date;
	error: string;
};
