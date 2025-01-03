import { CogIcon, Puzzle, User, Wrench } from "lucide-react";

import { Step } from "~/lib/model/workflows";

import { AgentSelectModule } from "~/components/agent/shared/AgentSelect";
import { BasicToolForm } from "~/components/tools/BasicToolForm";
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "~/components/ui/accordion";
import { Input } from "~/components/ui/input";
import { Switch } from "~/components/ui/switch";
import { WorkflowSelectModule } from "~/components/workflow/WorkflowSelectModule";

export function StepContent({
    step,
    onUpdate,
}: {
    step: Step;
    onUpdate: (updatedStep: Step) => void;
}) {
    return (
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
                            value={step.tools}
                            onChange={(tools) => onUpdate({ ...step, tools })}
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
                            onChange={(agents) => onUpdate({ ...step, agents })}
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
                                        temperature: parseFloat(e.target.value),
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
    );
}
