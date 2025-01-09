import { EditIcon, PlusIcon } from "lucide-react";
import { useState } from "react";

import { EmailReceiver } from "~/lib/model/email-receivers";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { EmailReceiverForm } from "~/components/workflow-triggers/EmailReceiverForm";

export function WorkflowEmailDialog({
    workflowId,
    emailReceiver,
}: {
    workflowId: string;
    emailReceiver?: EmailReceiver;
}) {
    const [open, setOpen] = useState(false);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                {emailReceiver ? (
                    <Button variant="ghost" size="icon">
                        <EditIcon />
                    </Button>
                ) : (
                    <Button variant="ghost" startContent={<PlusIcon />}>
                        Add Email Trigger
                    </Button>
                )}
            </DialogTrigger>
            <DialogContent className="p-0 gap-0">
                <DialogHeader className="p-8 pb-0">
                    <DialogTitle>
                        {emailReceiver
                            ? "Update Workflow Email Receiver"
                            : "Add Email Receiver To Workflow"}
                    </DialogTitle>

                    <DialogDescription>
                        Email Receivers are used to run the workflow when an
                        email is received.
                    </DialogDescription>
                </DialogHeader>

                <ScrollArea className="max-h-[60vh]">
                    <EmailReceiverForm
                        onContinue={() => setOpen(false)}
                        emailReceiver={
                            emailReceiver ?? { workflow: workflowId }
                        }
                        hideTitle
                    />
                </ScrollArea>
            </DialogContent>
        </Dialog>
    );
}
