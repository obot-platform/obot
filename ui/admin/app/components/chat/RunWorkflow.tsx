import { ComponentProps, useState } from "react";
import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { cn } from "~/lib/utils";

import { RunWorkflowForm } from "~/components/chat/RunWorkflowForm";
import { ModelProviderTooltip } from "~/components/model-providers/ModelProviderTooltip";
import { Button, ButtonProps } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

type RunWorkflowProps = {
    onSubmit: (params?: Record<string, string>) => void;
    workflowId: string;
    popoverContentProps?: ComponentProps<typeof PopoverContent>;
};

export function RunWorkflow({
    workflowId,
    onSubmit,
    popoverContentProps,
    ...props
}: RunWorkflowProps & ButtonProps) {
    const [open, setOpen] = useState(false);
    const { configured: modelProviderConfigured } = useModelProviders();
    const { data: workflow, isLoading } = useSWR(
        WorkflowService.getWorkflowById.key(workflowId),
        ({ workflowId }) => WorkflowService.getWorkflowById(workflowId)
    );

    const params = workflow?.params;

    if (!params || isLoading || !modelProviderConfigured)
        return (
            <ModelProviderTooltip enabled={modelProviderConfigured}>
                <Button
                    onClick={() => onSubmit()}
                    {...props}
                    disabled={
                        props.disabled || isLoading || !modelProviderConfigured
                    }
                    loading={isLoading || props.loading}
                >
                    Run Workflow
                </Button>
            </ModelProviderTooltip>
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
                {...popoverContentProps}
                className={cn("min-w-full", popoverContentProps?.className)}
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
