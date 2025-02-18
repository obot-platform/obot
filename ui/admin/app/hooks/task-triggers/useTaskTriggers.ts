import { useEffect, useState } from "react";
import useSWR from "swr";

import { TaskTriggerType, collateTaskTriggers } from "~/lib/model/task-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

type UseTaskTriggersProps = {
	type?: TaskTriggerType | TaskTriggerType[];
	taskId?: string;
};

const AllTypes = Object.values(TaskTriggerType);

export function useTaskTriggers(props?: UseTaskTriggersProps) {
	const [blockPollingEmailReceivers, setBlockPollingEmailReceivers] =
		useState(false);
	const { type = AllTypes, taskId } = props ?? {};

	const filters = { taskId };

	const types = new Set(Array.isArray(type) ? type : [type]);

	const { data: emailReceivers } = useSWR(
		types.has("email") &&
			EmailReceiverApiService.getEmailReceivers.key(filters),
		({ filters }) => EmailReceiverApiService.getEmailReceivers(filters),
		{
			fallbackData: [],
			refreshInterval: blockPollingEmailReceivers ? undefined : 1000,
		}
	);

	useEffect(() => {
		if (
			emailReceivers &&
			emailReceivers.some(
				(receiver) => receiver.aliasAssigned == null && receiver.alias
			)
		) {
			setBlockPollingEmailReceivers(false);
		} else {
			setBlockPollingEmailReceivers(true);
		}
	}, [emailReceivers]);

	const { data: cronjobs } = useSWR(
		...CronJobApiService.getCronJobs.swr(
			{ filters },
			{ enabled: types.has("schedule") }
		),
		{ fallbackData: [] }
	);

	const { data: webhooks } = useSWR(
		types.has("webhook") && WebhookApiService.getWebhooks.key(filters),
		({ filters }) => WebhookApiService.getWebhooks(filters),
		{ fallbackData: [] }
	);

	return {
		taskTriggers: getFilteredTriggers(),
		emailReceivers,
		cronjobs,
		webhooks,
	};

	function getFilteredTriggers() {
		const taskTriggers = collateTaskTriggers(
			[
				types.has("email") && emailReceivers,
				types.has("schedule") && cronjobs,
				types.has("webhook") && webhooks,
			]
				.filter((x) => !!x)
				.flat()
		);

		if (taskId) {
			return taskTriggers.filter((x) => x.task === taskId);
		}

		return taskTriggers;
	}
}
