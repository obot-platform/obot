import { EditIcon, PlusIcon } from "lucide-react";
import { useState } from "react";

import { Webhook, WebhookBase } from "~/lib/model/webhooks";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { WebhookForm } from "~/components/webhooks/WebhookForm";

export function WorkflowWebhookDialog({
    workflowId,
    webhook,
}: {
    workflowId: string;
    webhook?: WebhookBase;
}) {
    const [open, setOpen] = useState(false);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger>
                {webhook ? (
                    <Button variant="ghost" size="icon">
                        <EditIcon />
                    </Button>
                ) : (
                    <Button variant="ghost" startContent={<PlusIcon />}>
                        Add Webhook
                    </Button>
                )}
            </DialogTrigger>
            <DialogContent className="p-0 gap-0">
                <DialogHeader className="p-8 pb-0">
                    <DialogTitle>Add Webhook To Workflow</DialogTitle>
                </DialogHeader>

                <ScrollArea className="h-[600px]">
                    <WebhookForm
                        hideTitle
                        onContinue={() => setOpen(false)}
                        webhook={
                            { workflow: workflowId, ...webhook } as Webhook
                        }
                    />
                </ScrollArea>
            </DialogContent>
        </Dialog>
    );
}
