import { toast } from "sonner";
import { mutate } from "swr";

import { WorkflowTriggerType } from "~/lib/model/workflow-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { DropdownMenuItem } from "~/components/ui/dropdown-menu";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";

export function DeleteWorkflowTrigger({
    id,
    name,
    type,
}: {
    id: string;
    name?: string;
    type: WorkflowTriggerType;
}) {
    const deleteWebhook = useAsync(WebhookApiService.deleteWebhook, {
        onSuccess: () => {
            mutate(WebhookApiService.getWebhooks.key());
            toast.success("Webhook workflow trigger has been deleted.");
        },
    });

    const deleteCronjob = useAsync(CronJobApiService.deleteCronJob, {
        onSuccess: () => {
            mutate(CronJobApiService.getCronJobs.key());
            toast.success("Schedule workflow trigger has been deleted.");
        },
    });

    const deleteEmailReceiver = useAsync(
        EmailReceiverApiService.deleteEmailReceiver,
        {
            onSuccess: () => {
                mutate(EmailReceiverApiService.getEmailReceivers.key());
                toast.success("Email workflow trigger has been deleted.");
            },
        }
    );

    const { interceptAsync, dialogProps } = useConfirmationDialog();

    const handleConfirmDelete = async () =>
        await getDeleteFunction().executeAsync(id);

    return (
        <>
            <DropdownMenuItem
                variant="destructive"
                onClick={(e) => {
                    e.preventDefault();
                    interceptAsync(handleConfirmDelete);
                }}
            >
                Delete
            </DropdownMenuItem>

            <ConfirmationDialog
                {...dialogProps}
                title="Delete Workflow Trigger?"
                description={
                    <div className="flex flex-col">
                        <p>
                            Are you sure you want to delete workflow trigger:{" "}
                            <b>{name || id}</b>?
                        </p>
                        <p>The action cannot be undone.</p>
                    </div>
                }
                confirmProps={{
                    children: "Delete",
                    variant: "destructive",
                }}
            />
        </>
    );

    function getDeleteFunction() {
        switch (type) {
            case "webhook":
                return deleteWebhook;
            case "schedule":
                return deleteCronjob;
            case "email":
                return deleteEmailReceiver;
            default:
                throw new Error(`Unknown workflow trigger type: ${type}`);
        }
    }
}
