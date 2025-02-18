import { EmailReceiver } from "~/lib/model/email-receivers";

import { CopyText } from "~/components/composed/CopyText";
import { DeleteTaskTrigger } from "~/components/task-triggers/DeleteTaskTrigger";
import { TaskEmailDialog } from "~/components/task/triggers/TaskEmailDialog";

export function EmailTriggerEntry({
	receiver,
	taskId,
}: {
	receiver: EmailReceiver;
	taskId: string;
}) {
	return (
		<div key={receiver.id} className="flex items-center justify-between">
			<p>{receiver.name || receiver.id}</p>

			<div className="flex gap-2">
				<CopyText
					text={receiver.emailAddress ?? ""}
					className="bg-transparent text-sm text-muted-foreground"
					classNames={{
						text: "p-0",
					}}
					hideIcon
				/>

				<TaskEmailDialog taskId={taskId} emailReceiver={receiver} />

				<DeleteTaskTrigger type="email" id={receiver.id} />
			</div>
		</div>
	);
}
