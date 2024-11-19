import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";

const action = async (id: string) => {
    await WebhookApiService.deleteWebhook(id);
    await mutate(WebhookApiService.getWebhooks.key());
};

export function DeleteWebhook({
    id,
    children,
}: {
    id: string;
    children?: React.ReactNode;
}) {
    const [open, setOpen] = useState(false);

    const deleteWebhook = useAsync(action, {
        onSuccess: () => {
            toast.success("Webhook deleted");
            setOpen(false);
        },
    });

    return (
        <ConfirmationDialog
            open={open}
            onOpenChange={setOpen}
            title="Delete Webhook?"
            description="This action cannot be undone."
            onConfirm={() => deleteWebhook.execute(id)}
            closeOnConfirm={false}
            confirmProps={{
                loading: deleteWebhook.isLoading,
                disabled: deleteWebhook.isLoading,
                variant: "destructive",
                children: "Delete",
            }}
        >
            {children}
        </ConfirmationDialog>
    );
}
