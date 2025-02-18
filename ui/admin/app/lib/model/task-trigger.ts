import { CronJob } from "~/lib/model/cronjobs";
import { EmailReceiver } from "~/lib/model/email-receivers";
import { Webhook } from "~/lib/model/webhooks";

type TaskTriggerEntity = EmailReceiver | Webhook | CronJob;

export const TaskTriggerType = {
	Webhook: "webhook",
	Schedule: "schedule",
	Email: "email",
} as const;
export type TaskTriggerType =
	(typeof TaskTriggerType)[keyof typeof TaskTriggerType];

export type TaskTrigger = {
	id: string;
	type: TaskTriggerType;
	name: string;
	task: string;
};

const objectHasAllKeys = <T extends object = object>(
	obj: object,
	keys: (keyof T)[]
): obj is T => keys.every((key) => key in obj);

function isEmailReceiver(entity: TaskTriggerEntity): entity is EmailReceiver {
	return (
		entity.type === "emailreceiver" &&
		objectHasAllKeys<EmailReceiver>(entity, ["workflow"])
	);
}

function isWebhook(entity: TaskTriggerEntity): entity is Webhook {
	return objectHasAllKeys<Webhook>(entity, ["workflow", "validationHeader"]);
}

function isCronJob(entity: TaskTriggerEntity): entity is CronJob {
	return objectHasAllKeys<CronJob>(entity, ["workflow", "schedule"]);
}

function convertToTaskTrigger(entity: TaskTriggerEntity): TaskTrigger | null {
	switch (true) {
		case isEmailReceiver(entity):
			return {
				id: entity.id,
				type: "email",
				name: entity.name,
				task: entity.workflow,
			};
		case isWebhook(entity):
			return {
				id: entity.id,
				type: "webhook",
				name: entity.name,
				task: entity.workflow,
			};
		case isCronJob(entity):
			return {
				id: entity.id,
				type: "schedule",
				name: entity.id,
				task: entity.workflow,
			};
		default:
			console.error("Unknown entity type", entity);
			return null;
	}
}

export function collateTaskTriggers(list: TaskTriggerEntity[]) {
	return list.map(convertToTaskTrigger).filter((x) => !!x);
}
