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
    confirmProps,
    ...dialogProps
}: ComponentProps<typeof Dialog> & {
    children?: ReactNode;
    title: ReactNode;
    description?: ReactNode;
    onConfirm: () => void;
    onCancel?: () => void;
    confirmProps?: Omit<Partial<ComponentProps<typeof Button>>, "onClick">;
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

                    <DialogClose onClick={onConfirm} asChild>
                        <Button {...confirmProps}>
                            {confirmProps?.children ?? "Confirm"}
                        </Button>
                    </DialogClose>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
