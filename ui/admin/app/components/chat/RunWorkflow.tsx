import { useState } from "react";

import { RunWorkflowForm } from "~/components/chat/RunWorkflowForm";
import { Button, ButtonProps } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";

type RunWorkflowProps = {
    params?: Record<string, string>;
    onSubmit: (params?: Record<string, string>) => void;
};

export function RunWorkflow({
    params,
    onSubmit,
    disabled,
    ...props
}: RunWorkflowProps & ButtonProps) {
    const [open, setOpen] = useState(false);

    if (!params)
        return (
            <Button
                onClick={() => onSubmit()}
                {...props}
                disabled={open || disabled}
            >
                Run Workflow
            </Button>
        );

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button {...props} disabled={open || disabled}>
                    Run Workflow
                </Button>
            </PopoverTrigger>

            <PopoverContent className="min-w-full" side="bottom" align="center">
                <RunWorkflowForm
                    params={params}
                    onSubmit={(params) => {
                        setOpen(false);
                        onSubmit(params);
                    }}
                />
            </PopoverContent>
        </Popover>
    );
}
