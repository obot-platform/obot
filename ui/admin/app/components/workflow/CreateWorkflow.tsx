import { PlusIcon } from "lucide-react";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import { toast } from "sonner";
import { mutate } from "swr";

import { CapabilityTool } from "~/lib/model/toolReferences";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { generateRandomName } from "~/lib/service/nameGenerator";

import { Button } from "~/components/ui/button";
import { useAsync } from "~/hooks/useAsync";

const CapabilityTools = [
	CapabilityTool.Knowledge,
	CapabilityTool.WorkspaceFiles,
	CapabilityTool.Database,
];

export function CreateWorkflow() {
	const navigate = useNavigate();

	const createWorkflow = useAsync(WorkflowService.createWorkflow, {
		onSuccess: (res) => {
			mutate(WorkflowService.getWorkflows.key());
			toast.success("Workflow created");
			navigate($path("/workflows/:workflow", { workflow: res.id }));
		},
		onError: () => toast.error("Failed to create workflow"),
	});

	return (
		<Button
			variant="outline"
			startContent={<PlusIcon />}
			onClick={() =>
				createWorkflow.execute({
					name: generateRandomName(),
					tools: CapabilityTools,
				})
			}
			loading={createWorkflow.isLoading}
		>
			Create Workflow
		</Button>
	);
}
