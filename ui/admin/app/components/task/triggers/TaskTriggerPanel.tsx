import { WebhookIcon } from "lucide-react";

import { TaskTriggerType } from "~/lib/model/task-trigger";

import { TaskEmailTab } from "~/components/task/triggers/TaskEmailTab";
import { TaskScheduleTab } from "~/components/task/triggers/TaskScheduleTab";
import { TaskWebhookTab } from "~/components/task/triggers/TaskWebhookTab";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs";

export function TaskTriggerPanel({ taskId }: { taskId: string }) {
	return (
		<div className="m-4 flex flex-col gap-4 p-4">
			<Tabs defaultValue={TaskTriggerType.Schedule}>
				<div className="flex items-center justify-between">
					<h4 className="flex items-center gap-2">
						<WebhookIcon className="h-4 w-4" />
						Triggers
					</h4>

					<TabsList>
						<TabsTrigger value={TaskTriggerType.Schedule}>Schedule</TabsTrigger>

						<TabsTrigger value={TaskTriggerType.Email}>Email</TabsTrigger>

						<TabsTrigger value={TaskTriggerType.Webhook}>Webhook</TabsTrigger>
					</TabsList>
				</div>

				<TabsContent value={TaskTriggerType.Schedule}>
					<TaskScheduleTab taskId={taskId} />
				</TabsContent>

				<TabsContent value={TaskTriggerType.Email}>
					<TaskEmailTab taskId={taskId} />
				</TabsContent>

				<TabsContent value={TaskTriggerType.Webhook}>
					<TaskWebhookTab taskId={taskId} />
				</TabsContent>
			</Tabs>
		</div>
	);
}
