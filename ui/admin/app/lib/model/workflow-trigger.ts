import { CronJob } from "~/lib/model/cronjobs";
import { EmailReceiver } from "~/lib/model/email-receivers";
import { Webhook } from "~/lib/model/webhooks";

type WorkFlowTriggerEntity = EmailReceiver | Webhook | CronJob;

export const WorkflowTriggerType = {
	Webhook: "webhook",
	Schedule: "schedule",
	Email: "email",
} as const;
export type WorkflowTriggerType =
	(typeof WorkflowTriggerType)[keyof typeof WorkflowTriggerType];

export type WorkflowTrigger = {
	id: string;
	type: WorkflowTriggerType;
	name: string;
	workflow: string;
};

const objectHasAllKeys = <T extends object = object>(
	obj: object,
	keys: (keyof T)[]
): obj is T => keys.every((key) => key in obj);

function isEmailReceiver(
	entity: WorkFlowTriggerEntity
): entity is EmailReceiver {
	return objectHasAllKeys<EmailReceiver>(entity, ["workflow", "emailAddress"]);
}

function isWebhook(entity: WorkFlowTriggerEntity): entity is Webhook {
	return objectHasAllKeys<Webhook>(entity, ["workflow", "validationHeader"]);
}

function isCronJob(entity: WorkFlowTriggerEntity): entity is CronJob {
	return objectHasAllKeys<CronJob>(entity, ["workflow", "schedule"]);
}

function convertToWorkflowTrigger(
	entity: WorkFlowTriggerEntity
): WorkflowTrigger | null {
	switch (true) {
		case isEmailReceiver(entity):
			return {
				id: entity.id,
				type: "email",
				name: entity.name,
				workflow: entity.workflow,
			};
		case isWebhook(entity):
			return {
				id: entity.id,
				type: "webhook",
				name: entity.name,
				workflow: entity.workflow,
			};
		case isCronJob(entity):
			return {
				id: entity.id,
				type: "schedule",
				name: entity.id,
				workflow: entity.workflow,
			};
		default:
			console.error("Unknown entity type", entity);
			return null;
	}
}

export function collateWorkflowTriggers(list: WorkFlowTriggerEntity[]) {
	return list.map(convertToWorkflowTrigger).filter((x) => !!x);
}
