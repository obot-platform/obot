import {
	BlocksIcon,
	Library,
	List,
	PuzzleIcon,
	WrenchIcon,
} from "lucide-react";
import { useCallback, useState } from "react";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";

import { AssistantNamespace } from "~/lib/model/assistants";
import { CapabilityTool } from "~/lib/model/toolReferences";
import { Workflow as WorkflowType } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { AgentForm } from "~/components/agent";
import { AgentCapabilityForm } from "~/components/agent/shared/AgentCapabilityForm";
import { EnvironmentVariableSection } from "~/components/agent/shared/EnvironmentVariableSection";
import { ToolAuthenticationStatus } from "~/components/agent/shared/ToolAuthenticationStatus";
import { AgentKnowledgePanel } from "~/components/knowledge";
import { BasicToolForm } from "~/components/tools/BasicToolForm";
import { CardDescription } from "~/components/ui/card";
import { ScrollArea } from "~/components/ui/scroll-area";
import { DeleteWorkflowButton } from "~/components/workflow/DeleteWorkflow";
import { ParamsForm } from "~/components/workflow/ParamsForm";
import {
	WorkflowProvider,
	useWorkflow,
} from "~/components/workflow/WorkflowContext";
import { StepsForm } from "~/components/workflow/steps/StepsForm";
import { WorkflowTriggerPanel } from "~/components/workflow/triggers/WorkflowTriggerPanel";
import { useDebounce } from "~/hooks/useDebounce";

type WorkflowProps = {
	workflow: WorkflowType;
	onPersistThreadId: (threadId: string) => void;
	className?: string;
};

export function Workflow(props: WorkflowProps) {
	return (
		<WorkflowProvider workflow={props.workflow}>
			<WorkflowContent {...props} />
		</WorkflowProvider>
	);
}

function WorkflowContent({ className }: WorkflowProps) {
	const navigate = useNavigate();
	const { workflow, updateWorkflow, isUpdating, lastUpdated, refreshWorkflow } =
		useWorkflow();

	const [workflowUpdates, setWorkflowUpdates] = useState(workflow);

	const debouncedUpdateWorkflow = useDebounce(updateWorkflow, 1000);

	const partialSetWorkflow = useCallback(
		(changes: Partial<typeof workflow>) => {
			const updatedWorkflow = {
				...workflow,
				...workflowUpdates,
				...changes,
			};

			debouncedUpdateWorkflow(updatedWorkflow);

			setWorkflowUpdates(updatedWorkflow);
		},
		[debouncedUpdateWorkflow, workflow, workflowUpdates]
	);

	return (
		<div className="flex h-full flex-col">
			<ScrollArea className={cn("h-full", className)}>
				<div className="flex justify-end px-8 pt-4">
					<DeleteWorkflowButton
						id={workflow.id}
						onSuccess={() => navigate($path("/workflows"))}
					/>
				</div>
				<div className="m-4 px-4 pb-4">
					<AgentForm agent={workflowUpdates} onChange={partialSetWorkflow} />
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<BlocksIcon className="h-5 w-5" />
						Capabilities
					</h4>

					<CardDescription>
						Capabilities define core functions that enable the workflow to
						perform specialized tasks.
					</CardDescription>

					<AgentCapabilityForm
						entity={workflowUpdates}
						onChange={partialSetWorkflow}
						exclude={[CapabilityTool.Tasks]}
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<WrenchIcon className="h-5 w-5" />
						Tools
					</h4>

					<CardDescription>
						Add tools that allow the agent to perform useful actions such as
						searching the web, reading files, or interacting with other systems.
					</CardDescription>

					<BasicToolForm
						value={workflow.tools}
						onChange={(tools) => partialSetWorkflow({ tools })}
						renderActions={(tool) => (
							<ToolAuthenticationStatus
								namespace={AssistantNamespace.Workflows}
								entityId={workflow.id}
								tool={tool}
								toolInfo={workflow.toolInfo?.[tool]}
								onUpdate={refreshWorkflow}
							/>
						)}
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<List className="h-4 w-4" />
						Parameters
					</h4>

					<ParamsForm
						workflow={workflow}
						onChange={(values) =>
							partialSetWorkflow({
								params: values.params,
							})
						}
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<PuzzleIcon className="h-4 w-4" />
						Steps
					</h4>

					<StepsForm
						workflow={workflowUpdates}
						onChange={(values) => partialSetWorkflow({ steps: values.steps })}
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<Library className="h-4 w-4" />
						Knowledge
					</h4>

					<CardDescription>
						Provide knowledge to the workflow in the form of files, websites, or
						external links in order to give it context about various topics.
					</CardDescription>

					<AgentKnowledgePanel
						agent={workflowUpdates}
						agentId={workflow.id}
						updateAgent={partialSetWorkflow}
						addTool={(tool) => {
							if (workflow.tools?.includes(tool)) return;

							partialSetWorkflow({
								tools: [...(workflow.tools || []), tool],
							});
						}}
					/>
				</div>

				<WorkflowTriggerPanel workflowId={workflow.id} />

				<EnvironmentVariableSection
					entity={workflow}
					entityType="workflow"
					onUpdate={partialSetWorkflow}
				/>

				<div className="h-8" /* spacer */ />
			</ScrollArea>

			<footer className="flex items-center justify-between gap-4 border-t p-4 text-muted-foreground">
				{isUpdating ? <p>Saving...</p> : lastUpdated ? <p>Saved</p> : <div />}
			</footer>
		</div>
	);
}
