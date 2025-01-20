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
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";
import { useWorkflowTriggers } from "~/hooks/workflow-triggers/useWorkflowTriggers";

type DeleteWorkflowButtonProps = {
	id: string;
	onSuccess?: () => void;
};

export function DeleteWorkflowButton({
	id,
	onSuccess,
}: DeleteWorkflowButtonProps) {
	const deleteAssociatedTriggersConfirm = useConfirmationDialog();
	const deleteWorkflowConfirm = useConfirmationDialog();

	const { workflowTriggers } = useWorkflowTriggers({ workflowId: id });

	const deleteWorkflow = useAsync(WorkflowService.deleteWorkflow, {
		onSuccess: () => {
			mutate(WorkflowService.getWorkflows.key());
			toast.success("Workflow deleted");
			onSuccess?.();
		},
		onError: () => toast.error("Failed to delete workflow"),
	});

	const handleDelete = async (deleteTriggers: boolean) => {
		await deleteWorkflow.execute(id, deleteTriggers);
	};

	const handleConfirmDeleteWorkflow = () => {
		const handleConfirm = async () => {
			if (workflowTriggers.length > 0) {
				deleteAssociatedTriggersConfirm.interceptAsync(
					async () => handleDelete(true),
					{
						onCancel: async () => handleDelete(false),
					}
				);
			} else {
				handleDelete(false);
			}
		};

		deleteWorkflowConfirm.interceptAsync(handleConfirm);
	};

	return (
		<>
			<Tooltip>
				<TooltipTrigger onClick={(e) => e.stopPropagation()} asChild>
					<Button
						variant="ghost"
						size="icon"
						loading={deleteWorkflow.isLoading}
						onClick={handleConfirmDeleteWorkflow}
					>
						<TrashIcon />
					</Button>
				</TooltipTrigger>

				<TooltipContent>Delete Workflow</TooltipContent>
			</Tooltip>
			<ConfirmationDialog
				{...deleteWorkflowConfirm.dialogProps}
				title="Delete Workflow?"
				confirmProps={{ variant: "destructive", children: "Delete" }}
				description="This action cannot be undone."
			/>
			<ConfirmationDialog
				{...deleteAssociatedTriggersConfirm.dialogProps}
				title="Delete Associated Triggers?"
				description="There are attached workflow triggers to this workflow. Would you like to delete them as well?"
				confirmProps={{
					variant: "destructive",
					children: "Delete",
				}}
				cancelProps={{
					children: "Keep",
				}}
			/>
		</>
	);
}
