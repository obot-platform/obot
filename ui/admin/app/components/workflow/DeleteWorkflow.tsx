import { TrashIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

type DeleteWorkflowButtonProps = {
	id: string;
};

export function DeleteWorkflowButton({ id }: DeleteWorkflowButtonProps) {
	const deleteWorkflow = useAsync(WorkflowService.deleteWorkflow, {
		onSuccess: () => {
			mutate(WorkflowService.getWorkflows.key());
			toast.success("Workflow deleted");
		},
		onError: () => toast.error("Failed to delete workflow"),
	});

	return (
		<Tooltip>
			<ConfirmationDialog
				title="Delete Workflow?"
				onConfirm={() => deleteWorkflow.execute(id)}
				confirmProps={{ variant: "destructive", children: "Delete" }}
				description="This action cannot be undone."
			>
				<TooltipTrigger onClick={(e) => e.stopPropagation()} asChild>
					<Button
						variant="ghost"
						size="icon"
						loading={deleteWorkflow.isLoading}
					>
						<TrashIcon />
					</Button>
				</TooltipTrigger>
			</ConfirmationDialog>

			<TooltipContent>Delete Workflow</TooltipContent>
		</Tooltip>
	);
}
