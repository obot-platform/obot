import { EmailTriggerEntry } from "~/components/task/triggers/EmailReceiverEntry";
import { TaskEmailDialog } from "~/components/task/triggers/TaskEmailDialog";
import { CardDescription } from "~/components/ui/card";
import { useTaskTriggers } from "~/hooks/task-triggers/useTaskTriggers";

export function TaskEmailTab({ taskId }: { taskId: string }) {
	const { emailReceivers } = useTaskTriggers({ taskId });

	return (
		<div className="flex flex-col gap-2">
			<CardDescription>
				Add Email Triggers to run the task when an email is received.
			</CardDescription>

			{emailReceivers.map((emailReceiver) => (
				<EmailTriggerEntry
					key={emailReceiver.id}
					receiver={emailReceiver}
					taskId={taskId}
				/>
			))}

			<div className="self-end">
				<TaskEmailDialog taskId={taskId} />
			</div>
		</div>
	);
}
