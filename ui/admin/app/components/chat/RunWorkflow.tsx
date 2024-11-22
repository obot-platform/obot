import { ComponentProps, useState } from "react";
import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";

import { RunWorkflowForm } from "~/components/chat/RunWorkflowForm";
import { Button, ButtonProps } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";

type RunWorkflowProps = {
    onSubmit: (params?: Record<string, string>) => void;
    workflowId: string;
    popoverContentProps?: ComponentProps<typeof PopoverContent>;
};

export function RunWorkflow({
    workflowId,
    onSubmit,
    ...props
}: RunWorkflowProps & ButtonProps) {
    const [open, setOpen] = useState(false);

    const { data: workflow, isLoading } = useSWR(
        WorkflowService.getWorkflowById.key(workflowId),
        ({ workflowId }) => WorkflowService.getWorkflowById(workflowId)
    );

    const params = workflow?.params;

    if (!params || isLoading)
        return (
            <Button
                onClick={() => onSubmit()}
                {...props}
                disabled={props.disabled || isLoading}
                loading={isLoading || props.loading}
            >
                Run Workflow
            </Button>
        );

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button
                    {...props}
                    disabled={props.disabled || open || isLoading}
                    loading={props.loading || isLoading}
                    onClick={() => setOpen((prev) => !prev)}
                >
                    Run Workflow
                </Button>
            </PopoverTrigger>

            <PopoverContent
                {...props.popoverContentProps}
                className="min-w-full"
            >
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
