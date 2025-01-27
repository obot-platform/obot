import { useEffect } from "react";
import useSWR from "swr";

import {
	WorkflowTriggerType,
	collateWorkflowTriggers,
} from "~/lib/model/workflow-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

type UseWorkflowTriggersProps = {
	type?: WorkflowTriggerType | WorkflowTriggerType[];
	workflowId?: string;
};

const AllTypes = Object.values(WorkflowTriggerType);

export function useWorkflowTriggers(props?: UseWorkflowTriggersProps) {
	const { type = AllTypes, workflowId } = props ?? {};

	const filters = { workflowId };

	const types = new Set(Array.isArray(type) ? type : [type]);

	const { data: emailReceivers, mutate: mutateEmailReceivers } = useSWR(
		types.has("email") &&
			EmailReceiverApiService.getEmailReceivers.key(filters),
		({ filters }) => EmailReceiverApiService.getEmailReceivers(filters),
		{ fallbackData: [] }
	);

	useEffect(() => {
		if (
			emailReceivers &&
			emailReceivers.some(
				(receiver) => receiver.aliasAssigned == null && receiver.alias
			)
		) {
			mutateEmailReceivers();
		}
	}, [emailReceivers, mutateEmailReceivers]);

	const { data: cronjobs } = useSWR(
		types.has("schedule") && CronJobApiService.getCronJobs.key(filters),
		({ filters }) => CronJobApiService.getCronJobs(filters),
		{ fallbackData: [] }
	);

	const { data: webhooks } = useSWR(
		types.has("webhook") && WebhookApiService.getWebhooks.key(filters),
		({ filters }) => WebhookApiService.getWebhooks(filters),
		{ fallbackData: [] }
	);

	return {
		workflowTriggers: getFilteredTriggers(),
		emailReceivers,
		cronjobs,
		webhooks,
	};

	function getFilteredTriggers() {
		const workflowTriggers = collateWorkflowTriggers(
			[
				types.has("email") && emailReceivers,
				types.has("schedule") && cronjobs,
				types.has("webhook") && webhooks,
			]
				.filter((x) => !!x)
				.flat()
		);

		if (workflowId) {
			return workflowTriggers.filter((x) => x.workflow === workflowId);
		}

		return workflowTriggers;
	}
}
