import { ComponentProps, ReactNode } from "react";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";

export function ConfirmationDialog({
    children,
    title,
    description,
    onConfirm,
    onCancel,
    ...dialogProps
}: ComponentProps<typeof Dialog> & {
    children?: ReactNode;
    title: ReactNode;
    description?: ReactNode;
    onConfirm: () => void;
    onCancel?: () => void;
}) {
    return (
        <Dialog {...dialogProps}>
            {children && <DialogTrigger asChild>{children}</DialogTrigger>}

            <DialogContent>
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription>{description}</DialogDescription>
                <DialogFooter>
                    <DialogClose onClick={onCancel} asChild>
                        <Button variant="secondary">Cancel</Button>
                    </DialogClose>

                    <Button onClick={onConfirm}>Confirm</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
