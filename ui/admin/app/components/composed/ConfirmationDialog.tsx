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
    closeOnConfirm = true,
    ...dialogProps
}: ComponentProps<typeof Dialog> & {
    children?: ReactNode;
    title: ReactNode;
    description?: ReactNode;
    onConfirm: (e: React.MouseEvent<HTMLButtonElement>) => void;
    onCancel?: (e: React.MouseEvent<HTMLButtonElement>) => void;
    confirmProps?: Omit<Partial<ComponentProps<typeof Button>>, "onClick">;
    closeOnConfirm?: boolean;
}) {
    return (
        <Dialog {...dialogProps}>
            {children && <DialogTrigger asChild>{children}</DialogTrigger>}

            <DialogContent onClick={(e) => e.stopPropagation()}>
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription>{description}</DialogDescription>
                <DialogFooter>
                    <DialogClose onClick={onCancel} asChild>
                        <Button variant="secondary">Cancel</Button>
                    </DialogClose>

                    {closeOnConfirm ? (
                        <DialogClose onClick={onConfirm} asChild>
                            <Button {...confirmProps}>
                                {confirmProps?.children ?? "Confirm"}
                            </Button>
                        </DialogClose>
                    ) : (
                        <Button {...confirmProps} onClick={onConfirm}>
                            {confirmProps?.children ?? "Confirm"}
                        </Button>
                    )}
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
