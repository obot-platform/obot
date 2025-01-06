import { toast } from "sonner";
import { mutate } from "swr";

import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { TypographyP } from "~/components/Typography";
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
    type: "webhook" | "schedule";
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

    const handleConfirmDelete = async () => {
        if (type === "webhook") {
            await deleteWebhook.executeAsync(id);
        } else {
            await deleteCronjob.executeAsync(id);
        }
    };

    const { interceptAsync, dialogProps } = useConfirmationDialog();

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
                        <TypographyP>
                            Are you sure you want to delete workflow trigger:{" "}
                            <b>{name || id}</b>?
                        </TypographyP>
                        <TypographyP>The action cannot be undone.</TypographyP>
                    </div>
                }
                confirmProps={{
                    children: "Delete",
                    variant: "destructive",
                }}
            />
        </>
    );
}
