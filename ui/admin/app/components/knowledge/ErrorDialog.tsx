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
            <DialogContent className="max-w-[850px]">
                <DialogHeader>
                    <DialogTitle>Error</DialogTitle>
                </DialogHeader>
                <DialogDescription className="w-[800px] max-h-[800px] overflow-x-auto break-words whitespace-normal text-destructive">
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
