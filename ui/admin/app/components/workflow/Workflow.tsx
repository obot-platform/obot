import { Library, List, PuzzleIcon, Variable, WrenchIcon } from "lucide-react";
import { useCallback, useState } from "react";

import { AssistantNamespace } from "~/lib/model/assistants";
import { Workflow as WorkflowType } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { AgentForm } from "~/components/agent";
import { EnvironmentVariableSection } from "~/components/agent/shared/EnvironmentVariableSection";
import { ToolAuthenticationStatus } from "~/components/agent/shared/ToolAuthenticationStatus";
import { AgentKnowledgePanel } from "~/components/knowledge";
import { BasicToolForm } from "~/components/tools/BasicToolForm";
import { CardDescription } from "~/components/ui/card";
import { ScrollArea } from "~/components/ui/scroll-area";
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
    const {
        workflow,
        updateWorkflow,
        isUpdating,
        lastUpdated,
        refreshWorkflow,
    } = useWorkflow();

    const [workflowUpdates, setWorkflowUpdates] = useState(workflow);

    const partialSetWorkflow = useCallback(
        (changes: Partial<typeof workflow>) => {
            const updatedWorkflow = {
                ...workflow,
                ...workflowUpdates,
                ...changes,
            };

            updateWorkflow(updatedWorkflow);

            setWorkflowUpdates(updatedWorkflow);
        },
        [updateWorkflow, workflow, workflowUpdates]
    );

    const debouncedSetWorkflowInfo = useDebounce(partialSetWorkflow, 1000);

    return (
        <div className="h-full flex flex-col">
            <ScrollArea className={cn("h-full", className)}>
                <div className="p-4 m-4">
                    <AgentForm
                        agent={workflowUpdates}
                        onChange={debouncedSetWorkflowInfo}
                    />
                </div>

                <div className="p-4 m-4 flex flex-col gap-4">
                    <h4 className="flex items-center gap-2">
                        <WrenchIcon className="w-5 h-5" />
                        Tools
                    </h4>

                    <CardDescription>
                        Add tools that allow the agent to perform useful actions
                        such as searching the web, reading files, or interacting
                        with other systems.
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

                <div className="p-4 m-4 flex flex-col gap-4">
                    <h4 className="flex items-center gap-2">
                        <Variable className="w-4 h-4" />
                        Environment Variables
                    </h4>

                    <EnvironmentVariableSection
                        entity={workflow}
                        entityType="workflow"
                        onUpdate={partialSetWorkflow}
                    />
                </div>

                <div className="p-4 m-4 flex flex-col gap-4">
                    <h4 className="flex items-center gap-2">
                        <List className="w-4 h-4" />
                        Parameters
                    </h4>

                    <ParamsForm
                        workflow={workflow}
                        onChange={(values) =>
                            debouncedSetWorkflowInfo({
                                params: values.params,
                            })
                        }
                    />
                </div>

                <div className="p-4 m-4 flex flex-col gap-4">
                    <h4 className="flex items-center gap-2">
                        <PuzzleIcon className="w-4 h-4" />
                        Steps
                    </h4>

                    <StepsForm
                        workflow={workflowUpdates}
                        onChange={(values) =>
                            debouncedSetWorkflowInfo({ steps: values.steps })
                        }
                    />
                </div>

                <div className="p-4 m-4 flex flex-col gap-4">
                    <h4 className="flex items-center gap-2">
                        <Library className="w-4 h-4" />
                        Knowledge
                    </h4>

                    <CardDescription>
                        Provide knowledge to the workflow in the form of files,
                        websites, or external links in order to give it context
                        about various topics.
                    </CardDescription>

                    <AgentKnowledgePanel
                        agent={workflowUpdates}
                        agentId={workflow.id}
                        updateAgent={debouncedSetWorkflowInfo}
                        addTool={(tool) => {
                            if (workflow.tools?.includes(tool)) return;

                            debouncedSetWorkflowInfo({
                                tools: [...(workflow.tools || []), tool],
                            });
                        }}
                    />
                </div>

                <WorkflowTriggerPanel workflowId={workflow.id} />
            </ScrollArea>

            <footer className="flex justify-between items-center p-4 gap-4 border-t text-muted-foreground">
                {isUpdating ? (
                    <p>Saving...</p>
                ) : lastUpdated ? (
                    <p>Saved</p>
                ) : (
                    <div />
                )}
            </footer>
        </div>
    );
}
