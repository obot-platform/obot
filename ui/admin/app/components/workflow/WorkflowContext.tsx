import {
	ReactNode,
	createContext,
	useCallback,
	useContext,
	useState,
} from "react";
import useSWR, { mutate } from "swr";

import { Workflow } from "~/lib/model/workflows";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { useAsync } from "~/hooks/useAsync";

interface WorkflowContextType {
	workflow: Workflow;
	workflowId: string;
	updateWorkflow: (workflow: Workflow) => void;
	refreshWorkflow: () => void;
	isUpdating: boolean;
	lastUpdated?: Date;
}

const WorkflowContext = createContext<WorkflowContextType | undefined>(
	undefined
);

export function WorkflowProvider({
	children,
	workflow: initialWorkflow,
}: {
	children: ReactNode;
	workflow: Workflow;
}) {
	const workflowId = initialWorkflow.id;

	const getWorkflow = useSWR(
		WorkflowService.getWorkflowById.key(workflowId),
		({ workflowId }) => WorkflowService.getWorkflowById(workflowId),
		{ fallbackData: initialWorkflow }
	);

	const [lastUpdated, setLastSaved] = useState<Date>();

	const handleUpdateWorkflow = useCallback(
		(updatedWorkflow: Workflow) =>
			WorkflowService.updateWorkflow({
				id: workflowId,
				workflow: updatedWorkflow,
			})
				.then((updatedWorkflow) => {
					getWorkflow.mutate(updatedWorkflow);
					mutate(WorkflowService.getWorkflows.key());
					setLastSaved(new Date());
				})
				.catch(console.error),
		[workflowId, getWorkflow]
	);

	const updateWorkflow = useAsync(handleUpdateWorkflow);

	const refreshWorkflow = getWorkflow.mutate;

	return (
		<WorkflowContext.Provider
			value={{
				workflowId,
				workflow: getWorkflow.data,
				updateWorkflow: updateWorkflow.execute,
				refreshWorkflow,
				isUpdating: updateWorkflow.isLoading,
				lastUpdated,
			}}
		>
			{children}
		</WorkflowContext.Provider>
	);
}

export function useWorkflow() {
	const context = useContext(WorkflowContext);
	if (context === undefined) {
		throw new Error("useChat must be used within a ChatProvider");
	}
	return context;
}
