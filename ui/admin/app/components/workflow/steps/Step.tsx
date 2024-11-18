import {
    ArrowRight,
    ChevronDown,
    ChevronRight,
    CogIcon,
    Puzzle,
    Trash,
    User,
    Wrench,
} from "lucide-react";
import { useState } from "react";

import { Step } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { AgentSelectModule } from "~/components/agent/shared/AgentSelect";
import { BasicToolForm } from "~/components/tools/BasicToolForm";
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { Input } from "~/components/ui/input";
import { Switch } from "~/components/ui/switch";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { WorkflowSelectModule } from "~/components/workflow/WorkflowSelectModule";

export function StepComponent({
    step,
    onUpdate,
    onDelete,
    className,
}: {
    step: Step;
    onUpdate: (updatedStep: Step) => void;
    onDelete: () => void;
    className?: string;
}) {
    const [isExpanded, setIsExpanded] = useState(false);

    return (
        <div className={cn("border rounded-md", className)}>
            <ClickableDiv
                className={cn(
                    "flex items-start p-3 bg-secondary",
                    isExpanded ? "rounded-t-md" : "rounded-md"
                )}
                onClick={() => setIsExpanded((prev) => !prev)}
            >
                <div className="flex items-center">
                    <Button
                        variant="ghost"
                        size="icon"
                        className="p-0 w-6 h-6 mr-2 self-center"
                    >
                        {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                        ) : (
                            <ChevronRight className="w-4 h-4" />
                        )}
                    </Button>

                    <div className="flex items-center justify-center p-2 w-24 bg-background-secondary border rounded-md mr-2">
                        <ArrowRight className="w-4 h-4 mr-1" />
                        <span className="text-sm font-medium">Step</span>
                    </div>
                </div>

                <AutosizeTextarea
                    value={step.step}
                    onChange={(e) =>
                        onUpdate({ ...step, step: e.target.value })
                    }
                    placeholder="Step"
                    maxHeight={100}
                    minHeight={0}
                    className="flex-grow bg-background-secondary"
                    onClick={(e) => e.stopPropagation()}
                />

                <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => {
                        e.stopPropagation();
                        onDelete();
                    }}
                    className="ml-2 min-w-fit"
                >
                    <Trash className="w-4 h-4" />
                </Button>
            </ClickableDiv>

            {isExpanded && (
                <div className="p-3 space-y-4 px-8">
                    <Accordion type="multiple">
                        <AccordionItem value="tools">
                            <AccordionTrigger>
                                <span className="flex items-center gap-2 justify-center">
                                    <Wrench className="w-4 h-4" />
                                    Tools
                                </span>
                            </AccordionTrigger>

                            <AccordionContent className="p-1 pb-6">
                                <BasicToolForm
                                    defaultValues={step}
                                    onChange={(values) =>
                                        onUpdate({ ...step, ...values })
                                    }
                                />
                            </AccordionContent>
                        </AccordionItem>

                        <AccordionItem value="workflows">
                            <AccordionTrigger>
                                <span className="flex items-center gap-2 justify-center">
                                    <Puzzle className="w-4 h-4" />
                                    Workflows
                                </span>
                            </AccordionTrigger>

                            <AccordionContent className="p-1 pb-6">
                                <WorkflowSelectModule
                                    onChange={(workflows) =>
                                        onUpdate({ ...step, workflows })
                                    }
                                    selection={step.workflows || []}
                                />
                            </AccordionContent>
                        </AccordionItem>

                        <AccordionItem value="agents">
                            <AccordionTrigger>
                                <span className="flex items-center gap-2 justify-center">
                                    <User className="w-4 h-4" />
                                    Agents
                                </span>
                            </AccordionTrigger>

                            <AccordionContent className="p-1 pb-6">
                                <AgentSelectModule
                                    onChange={(agents) =>
                                        onUpdate({ ...step, agents })
                                    }
                                    selection={step.agents || []}
                                />
                            </AccordionContent>
                        </AccordionItem>

                        <AccordionItem value="advanced">
                            <AccordionTrigger>
                                <span className="flex items-center gap-2 justify-center">
                                    <CogIcon className="w-4 h-4" />
                                    Advanced
                                </span>
                            </AccordionTrigger>

                            <AccordionContent className="p-1 pb-6 space-y-6">
                                <div>
                                    <label
                                        htmlFor="temperature"
                                        className="block text-sm font-medium text-gray-700 mb-1"
                                    >
                                        Temperature
                                    </label>

                                    <Input
                                        id="temperature"
                                        type="number"
                                        value={step.temperature}
                                        onChange={(e) =>
                                            onUpdate({
                                                ...step,
                                                temperature: parseFloat(
                                                    e.target.value
                                                ),
                                            })
                                        }
                                        placeholder="Temperature"
                                        className="bg-background"
                                    />
                                </div>

                                <div className="flex items-center space-x-2">
                                    <Switch
                                        checked={step.cache}
                                        onCheckedChange={(checked) =>
                                            onUpdate({
                                                ...step,
                                                cache: checked,
                                            })
                                        }
                                    />

                                    <span>Cache</span>
                                </div>
                            </AccordionContent>
                        </AccordionItem>
                    </Accordion>
                </div>
            )}
        </div>
    );
}
