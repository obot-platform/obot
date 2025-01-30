import { TrashIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { AgentService } from "~/lib/service/api/agentService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

export function DeleteAgent({
	id,
	onSuccess,
}: {
	id: string;
	onSuccess?: () => void;
}) {
	const deleteAgent = useAsync(AgentService.deleteAgent, {
		onSuccess: () => {
			toast.success("Agent deleted");
			mutate(AgentService.getAgents.key());
			onSuccess?.();
		},
		onError: () => toast.error("Failed to delete agent"),
	});

	return (
		<Tooltip>
			<TooltipContent>Delete Agent</TooltipContent>

			<ConfirmationDialog
				title="Delete Agent?"
				description="This action cannot be undone."
				onConfirm={() => deleteAgent.execute(id)}
				confirmProps={{ variant: "destructive" }}
			>
				<TooltipTrigger asChild>
					<Button
						variant="ghost"
						size="icon"
						disabled={deleteAgent.isLoading}
						loading={deleteAgent.isLoading}
					>
						<TrashIcon />
					</Button>
				</TooltipTrigger>
			</ConfirmationDialog>
		</Tooltip>
	);
}
