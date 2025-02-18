import { TrashIcon } from "lucide-react";
import { toast } from "sonner";

import { TaskTriggerType } from "~/lib/model/task-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";

type DeleteTriggerProps = {
	type: TaskTriggerType;
	id: string;
	children?: React.ReactNode;
};

export function DeleteTaskTrigger({ type, id, children }: DeleteTriggerProps) {
	const { delete: deleteAction, revalidate, label } = getActions(type);

	const deleteTrigger = useAsync(deleteAction, {
		onSuccess: () => {
			toast.success(`${label} has been deleted.`);
			revalidate();
		},
	});

	const { interceptAsync, dialogProps } = useConfirmationDialog();

	const handleDelete = () =>
		interceptAsync(() => deleteTrigger.executeAsync(id));

	return (
		<>
			{children ? (
				<ClickableDiv
					onClick={(e) => {
						e.stopPropagation();
						handleDelete();
					}}
				>
					{children}
				</ClickableDiv>
			) : (
				<Button size="icon" variant="ghost" onClick={handleDelete}>
					<TrashIcon />
				</Button>
			)}

			<ConfirmationDialog
				{...dialogProps}
				title={`Delete ${label}`}
				confirmProps={{ children: "Delete", variant: "destructive" }}
				description={`Are you sure you want to delete this ${label}? This action cannot be undone.`}
			/>
		</>
	);
}

const getActions = (type: TaskTriggerType) => {
	switch (type) {
		case "email":
			return {
				delete: EmailReceiverApiService.deleteEmailReceiver,
				revalidate: EmailReceiverApiService.getEmailReceivers.revalidate,
				label: "Email Trigger",
			};
		case "schedule":
			return {
				delete: CronJobApiService.deleteCronJob,
				revalidate: CronJobApiService.getCronJobs.revalidate,
				label: "Schedule Trigger",
			};
		case "webhook":
			return {
				delete: WebhookApiService.deleteWebhook,
				revalidate: WebhookApiService.getWebhooks.revalidate,
				label: "Webhook Trigger",
			};
	}
};
