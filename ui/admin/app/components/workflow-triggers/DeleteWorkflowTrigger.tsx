import { TrashIcon } from "lucide-react";
import { toast } from "sonner";

import { WorkflowTriggerType } from "~/lib/model/workflow-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";

type DeleteTriggerProps = {
    type: WorkflowTriggerType;
    id: string;
};

export function DeleteWorkflowTrigger({ type, id }: DeleteTriggerProps) {
    const { delete: deleteAction, revalidate, label } = getActions(type);

    const deleteTrigger = useAsync(deleteAction, {
        onSuccess: () => {
            toast.success(`${label} has been deleted.`);
            revalidate();
        },
    });

    const { interceptAsync, dialogProps } = useConfirmationDialog();

    return (
        <>
            <Button
                loading={deleteTrigger.isLoading}
                disabled={deleteTrigger.isLoading}
                size="icon"
                variant="ghost"
                onClick={() =>
                    interceptAsync(() => deleteTrigger.executeAsync(id))
                }
            >
                <TrashIcon />
            </Button>

            <ConfirmationDialog
                {...dialogProps}
                title={`Delete ${label}`}
                confirmProps={{ children: "Delete", variant: "destructive" }}
                description={`Are you sure you want to delete this ${label}? This action cannot be undone.`}
            />
        </>
    );
}

const getActions = (type: WorkflowTriggerType) => {
    switch (type) {
        case "email":
            return {
                delete: EmailReceiverApiService.deleteEmailReceiver,
                revalidate:
                    EmailReceiverApiService.getEmailReceivers.revalidate,
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
