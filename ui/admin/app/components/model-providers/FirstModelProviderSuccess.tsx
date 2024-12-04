import { CircleCheckIcon } from "lucide-react";

import { TypographyP } from "~/components/Typography";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";

export function FirstModelProviderSuccess({
    open,
    onClose,
}: {
    open: boolean;
    onClose: (open: boolean) => void;
}) {
    return (
        <Dialog open={open} onOpenChange={onClose}>
            <DialogContent
                classNames={{
                    content: "max-w-xs",
                }}
            >
                <DialogHeader>
                    <DialogTitle className="flex gap-1 items-center justify-center">
                        <CircleCheckIcon className="text-success" /> Success!
                    </DialogTitle>
                </DialogHeader>

                <div className="flex flex-col gap-4">
                    <TypographyP className="text-sm">
                        Your first model provider is set up!
                    </TypographyP>

                    <TypographyP className="text-sm">
                        You can now create agents, start workflows, and add
                        models with your model provider.
                    </TypographyP>
                </div>
            </DialogContent>
        </Dialog>
    );
}
