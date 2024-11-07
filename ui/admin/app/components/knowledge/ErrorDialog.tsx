import React from "react";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";

type ErrorDialogProps = {
    error: string;
    isOpen: boolean;
    onClose: () => void;
};

const ErrorDialog: React.FC<ErrorDialogProps> = ({
    error,
    isOpen,
    onClose,
}) => {
    return (
        <Dialog open={isOpen} onOpenChange={onClose}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Error</DialogTitle>
                </DialogHeader>
                <DialogDescription className="whitespace-normal overflow-y-auto break-words max-h-full text-destructive">
                    {error}
                </DialogDescription>
                <DialogFooter>
                    <Button onClick={onClose}>Close</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};

export default ErrorDialog;
