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

import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Switch } from "~/components/ui/switch";
import { Textarea } from "~/components/ui/textarea";
import { StringArrayForm } from "~/components/workflow/StringArrayForm";

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
            <div
                className={cn(
                    "flex items-center p-3 bg-secondary",
                    isExpanded ? "rounded-t-md" : "rounded-md"
                )}
            >
                <Button
                    variant="ghost"
                    size="sm"
                    className="p-0 w-6 h-6 mr-2"
                    onClick={() => setIsExpanded(!isExpanded)}
                >
                    {isExpanded ? (
                        <ChevronDown className="w-4 h-4" />
                    ) : (
                        <ChevronRight className="w-4 h-4" />
                    )}
                </Button>
                <div className="flex items-center justify-center w-24 h-[60.5px] border bg-background rounded-md mr-2">
                    <ArrowRight className="w-4 h-4 mr-1" />
                    <span className="text-sm font-medium">Step</span>
                </div>
                <Textarea
                    value={step.step}
                    onChange={(e) =>
                        onUpdate({ ...step, step: e.target.value })
                    }
                    placeholder="Step"
                    className="flex-grow bg-background"
                />
                <Button
                    variant="destructive"
                    size="icon"
                    onClick={onDelete}
                    className="ml-2"
                >
                    <Trash className="w-4 h-4" />
                </Button>
            </div>
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
                                <StringArrayForm
                                    initialItems={step.tools || []}
                                    onChange={(values) =>
                                        onUpdate({
                                            ...step,
                                            tools: values.items,
                                        })
                                    }
                                    itemName="Tool"
                                    placeholder="Add a tool"
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
                                <StringArrayForm
                                    initialItems={step.workflows || []}
                                    onChange={(values) =>
                                        onUpdate({
                                            ...step,
                                            workflows: values.items,
                                        })
                                    }
                                    itemName="Workflow"
                                    placeholder="Add a workflow"
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
                                <StringArrayForm
                                    initialItems={step.agents || []}
                                    onChange={(values) =>
                                        onUpdate({
                                            ...step,
                                            agents: values.items,
                                        })
                                    }
                                    itemName="Agent"
                                    placeholder="Add an agent"
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
