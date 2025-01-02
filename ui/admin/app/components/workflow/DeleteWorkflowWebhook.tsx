import { TrashIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { TypographyP } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { useAsync } from "~/hooks/useAsync";

export function DeleteWorkflowWebhook({ webhookId }: { webhookId: string }) {
    const [open, setOpen] = useState(false);

    const deleteWebhook = useAsync(WebhookApiService.deleteWebhook, {
        onSuccess: () => {
            mutate(WebhookApiService.getWebhooks.key());
        },
        onError: () => toast.error(`Something went wrong.`),
    });

    const handleDelete = async () => {
        await deleteWebhook.execute(webhookId);
        setOpen(false);
    };

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger>
                <Button variant="ghost" size="icon">
                    <TrashIcon />
                </Button>
            </PopoverTrigger>
            <PopoverContent>
                <TypographyP>
                    Are you sure you want to delete this webhook?
                </TypographyP>
                <div className="flex justify-end gap-2">
                    <Button variant="ghost" onClick={() => setOpen(false)}>
                        Cancel
                    </Button>
                    <Button
                        variant="destructive"
                        onClick={handleDelete}
                        loading={deleteWebhook.isLoading}
                    >
                        Delete
                    </Button>
                </div>
            </PopoverContent>
        </Popover>
    );
}
