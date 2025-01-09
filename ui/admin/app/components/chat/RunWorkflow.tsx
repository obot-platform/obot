import { SquareIcon } from "lucide-react";
import { ComponentProps, useState } from "react";
import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
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
	const { abortRunningThread, isInvoking, isRunning, messages } = useChat();

	const params = workflow?.params;

	const loading = props.loading || isLoading || isInvoking;
	const disabled = props.disabled || loading || !modelProviderConfigured;

	const latestMessage = messages?.[messages.length - 1];
	const authenticating =
		latestMessage?.prompt?.metadata?.authURL ||
		latestMessage?.prompt?.metadata?.authType;
	if (isRunning) {
		return (
			<Button
				onClick={() => abortRunningThread()}
				{...props}
				disabled={disabled}
				startContent={
					<SquareIcon className="!h-3 !w-3 fill-primary-foreground text-primary-foreground" />
				}
			>
				{authenticating ? "Authenticating" : "Stop Workflow"}
			</Button>
		);
	}

	if (!params || !modelProviderConfigured)
		return (
			<ModelProviderTooltip enabled={modelProviderConfigured}>
				<Button
					onClick={() => onSubmit()}
					{...props}
					disabled={disabled}
					loading={loading}
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
					disabled={disabled || open}
					loading={loading}
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
