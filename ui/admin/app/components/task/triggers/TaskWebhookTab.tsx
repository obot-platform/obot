import { TaskTriggerType } from "~/lib/model/task-trigger";

import { DeleteTaskTrigger } from "~/components/task-triggers/DeleteTaskTrigger";
import { TaskWebhookDialog } from "~/components/task/triggers/TaskWebhookDialog";
import { CardDescription } from "~/components/ui/card";
import { useTaskTriggers } from "~/hooks/task-triggers/useTaskTriggers";

export function TaskWebhookTab({ taskId }: { taskId: string }) {
	const { webhooks } = useTaskTriggers({ taskId });

	return (
		<div className="flex flex-col gap-4">
			<CardDescription>
				Add webhooks to notify external services when your AI agent completes
				tasks or receives new information.
			</CardDescription>

			<div className="flex flex-col gap-2">
				{webhooks?.map((webhook) => (
					<div key={webhook.id} className="flex justify-between">
						<p>{webhook.name || webhook.id}</p>

						<div className="flex gap-2">
							<TaskWebhookDialog taskId={taskId} webhook={webhook} />
							<DeleteTaskTrigger
								type={TaskTriggerType.Webhook}
								id={webhook.id}
							/>
						</div>
					</div>
				))}
			</div>

			<div className="flex justify-end">
				<TaskWebhookDialog taskId={taskId} />
			</div>
		</div>
	);
}
