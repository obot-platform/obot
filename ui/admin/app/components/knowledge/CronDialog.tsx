import cronstrue from "cronstrue";
import { FC, useState } from "react";

import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

interface CronDialogProps {
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    cronExpression: string;
    setCronExpression: (cronExpression: string) => void;
    onSubmit?: () => void;
}

const CronDialog: FC<CronDialogProps> = ({
    isOpen,
    onOpenChange,
    cronExpression,
    setCronExpression,
    onSubmit,
}) => {
    const [cronDescription, setCronDescription] = useState("");

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent aria-describedby={undefined} className="max-w-md">
                <DialogTitle className="text-xl font-semibold mb-4">
                    Enter Cron Expression
                </DialogTitle>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={cronExpression}
                        onChange={(e) => {
                            setCronExpression(e.target.value);
                            try {
                                setCronDescription(
                                    cronstrue.toString(e.target.value)
                                );
                            } catch (_) {
                                setCronDescription(
                                    "Enter a valid cron expression"
                                );
                            }
                        }}
                        placeholder="* * * * *"
                        className="w-full dark:bg-secondary"
                    />
                    <span className="block mt-2 text-sm text-gray-500">
                        {cronDescription}
                    </span>
                </div>
                <div className="flex justify-end gap-2">
                    <Button
                        onClick={() => {
                            onOpenChange(false);
                            if (onSubmit) {
                                onSubmit();
                            }
                        }}
                        className="w-full"
                        variant="secondary"
                    >
                        Ok
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};

export default CronDialog;
