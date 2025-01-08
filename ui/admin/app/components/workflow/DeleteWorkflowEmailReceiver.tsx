import { TrashIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";

import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { useAsync } from "~/hooks/useAsync";

export function DeleteWorkflowEmailReceiver({
    emailReceiverId,
}: {
    emailReceiverId: string;
}) {
    const [open, setOpen] = useState(false);

    const deleteEmailReceiver = useAsync(
        EmailReceiverApiService.deleteEmailReceiver,
        {
            onSuccess: () => {
                mutate(EmailReceiverApiService.getEmailReceivers.key());
            },
            onError: () => toast.error(`Something went wrong.`),
        }
    );

    const handleDelete = async () => {
        await deleteEmailReceiver.execute(emailReceiverId);
        setOpen(false);
    };

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button variant="ghost" size="icon">
                    <TrashIcon />
                </Button>
            </PopoverTrigger>

            <PopoverContent>
                <p>Are you sure you want to delete this email trigger?</p>
                <div className="flex justify-end gap-2">
                    <Button variant="ghost" onClick={() => setOpen(false)}>
                        Cancel
                    </Button>
                    <Button
                        variant="destructive"
                        onClick={handleDelete}
                        loading={deleteEmailReceiver.isLoading}
                    >
                        Delete
                    </Button>
                </div>
            </PopoverContent>
        </Popover>
    );
}
