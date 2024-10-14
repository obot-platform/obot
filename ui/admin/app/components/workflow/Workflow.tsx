// import { WorkflowKnowledgePanel } from "~/components/knowledge";
import {
    AlertCircle,
    CogIcon,
    List,
    PuzzleIcon,
    RotateCcw,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Workflow as WorkflowType } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { useChat } from "~/components/chat";
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import { WorkflowAdvancedForm } from "~/components/workflow/WorkflowAdvancedForm";
import {
    WorkflowProvider,
    useWorkflow,
} from "~/components/workflow/WorkflowContext";
import { WorkflowForm } from "~/components/workflow/WorkflowForm";
import { StepsForm } from "~/components/workflow/steps/StepsForm";
import { useDebounce } from "~/hooks/useDebounce";

import { ParamsForm } from "./ParamsForm";
import { StringArrayForm } from "./StringArrayForm";

type WorkflowProps = {
    workflow: WorkflowType;
    className?: string;
    onRefresh?: () => void;
};

export function Workflow(props: WorkflowProps) {
    return (
        <WorkflowProvider workflow={props.workflow}>
            <WorkflowContent {...props} />
        </WorkflowProvider>
    );
}

function WorkflowContent({ className, onRefresh }: WorkflowProps) {
    const { workflow, updateWorkflow, isUpdating, lastUpdated } = useWorkflow();
    const { invoke } = useChat();

    const runWorkflow = () => {
        onRefresh?.();
        invoke();
    };

    const [workflowUpdates, setWorkflowUpdates] = useState(workflow);

    useEffect(() => {
        setWorkflowUpdates(workflow);
    }, [workflow]);

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
        [workflowUpdates, updateWorkflow, workflow]
    );

    const debouncedSetWorkflowInfo = useDebounce(partialSetWorkflow, 1000);

    return (
        <div className="h-full flex flex-col">
            <ScrollArea className={cn("h-full", className)}>
                <div className="p-4">
                    <WorkflowForm
                        workflow={workflow}
                        onChange={debouncedSetWorkflowInfo}
                    />
                </div>

                <Accordion type="multiple" className="p-4 flex-auto">
                    <AccordionItem value="params-form">
                        <AccordionTrigger>
                            <span className="flex items-center gap-2 justify-center">
                                <List className="w-4 h-4" />
                                Parameters
                            </span>
                        </AccordionTrigger>
                        <AccordionContent className="p-2">
                            <ParamsForm
                                workflow={workflow}
                                onChange={(values) =>
                                    partialSetWorkflow({
                                        params: values.params,
                                    })
                                }
                            />
                        </AccordionContent>
                    </AccordionItem>
                    <AccordionItem value="steps-form">
                        <AccordionTrigger>
                            <span className="flex items-center gap-2 justify-center">
                                <PuzzleIcon className="w-4 h-4" />
                                Steps
                            </span>
                        </AccordionTrigger>
                        <AccordionContent className="p-2">
                            <StepsForm
                                workflow={workflow}
                                onChange={(values) =>
                                    partialSetWorkflow({ steps: values.steps })
                                }
                            />
                        </AccordionContent>
                    </AccordionItem>
                    <AccordionItem value="advanced-form">
                        <AccordionTrigger>
                            <span className="flex items-center gap-2 justify-center">
                                <CogIcon className="w-4 h-4" />
                                Advanced
                            </span>
                        </AccordionTrigger>
                        <AccordionContent className="p-2 space-y-8">
                            <p className="text-muted-foreground text-sm bg-accent p-4 rounded-md">
                                <AlertCircle className="w-4 h-4 mr-2 inline" />
                                These settings edit the root of the workflow and
                                thus changing anything here will affect all
                                steps in the workflow.
                            </p>
                            <WorkflowAdvancedForm
                                workflow={workflow}
                                onChange={partialSetWorkflow}
                            />
                            <div>
                                <p className="text-default font-medium mb-2">
                                    Workflow tools
                                </p>
                                <StringArrayForm
                                    initialItems={workflow.tools || []}
                                    onChange={(values) =>
                                        partialSetWorkflow({
                                            tools: values.items,
                                        })
                                    }
                                    itemName="Tool"
                                    placeholder="Add a tool"
                                />
                            </div>
                        </AccordionContent>
                    </AccordionItem>
                </Accordion>
            </ScrollArea>

            <footer className="flex justify-between items-center p-4 gap-4 text-muted-foreground">
                {isUpdating ? (
                    <TypographyP>Saving...</TypographyP>
                ) : lastUpdated ? (
                    <TypographyP>Saved</TypographyP>
                ) : (
                    <div />
                )}

                <Button
                    className="flex gap-2"
                    variant="secondary"
                    onClick={runWorkflow}
                >
                    <RotateCcw className="w-4 h-4" /> Run Workflow
                </Button>
            </footer>
        </div>
    );
}
