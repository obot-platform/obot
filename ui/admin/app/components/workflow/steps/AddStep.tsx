import { ArrowRight, GitFork, Plus, RotateCw } from "lucide-react";
import { useMemo, useState } from "react";
import useSWR from "swr";

import {
    ToolReference,
    toolReferenceToTemplate,
} from "~/lib/model/toolReferences";
import { Step, getDefaultStep } from "~/lib/model/workflows";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { ScrollArea } from "~/components/ui/scroll-area";
import { StepTemplateCard } from "~/components/workflow/steps/StepTemplateCard";

type StepType = "regular" | "if" | "while" | "template";

interface AddStepButtonProps {
    onAddStep: (newStep: Step) => void;
    className?: string;
}

export function AddStepButton({ onAddStep, className }: AddStepButtonProps) {
    const [open, setOpen] = useState(false);
    const [isTemplateModalOpen, setIsTemplateModalOpen] = useState(false);

    const getStepTemplates = useSWR(
        ToolReferenceService.getToolReferences.key("stepTemplate"),
        () => ToolReferenceService.getToolReferences("stepTemplate")
    );

    const stepTemplates = useMemo(() => {
        if (!getStepTemplates.data) return [];
        return getStepTemplates.data;
    }, [getStepTemplates.data]);

    const createNewStep = (type: StepType) => {
        if (type === "template") {
            setIsTemplateModalOpen(true);
        } else {
            const newStep = getDefaultStep();

            if (type === "if") {
                newStep.if = { condition: "", steps: [], else: [] };
            } else if (type === "while") {
                newStep.while = { condition: "", maxLoops: 0, steps: [] };
            }

            onAddStep(newStep);
            setOpen(false);
        }
    };

    const handleStepTemplateSelection = (stepTemplate: ToolReference) => {
        const newStep: Step = {
            id: Date.now().toString(),
            name: stepTemplate.name,
            description: "",
            template: toolReferenceToTemplate(stepTemplate),
            cache: false,
            temperature: 0,
            tools: [],
            agents: [],
            workflows: [],
        };
        onAddStep(newStep);
        setIsTemplateModalOpen(false);
    };

    return (
        <>
            <Popover open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>
                    <Button variant="ghost" className={className}>
                        <Plus /> Add Step
                    </Button>
                </PopoverTrigger>

                <PopoverContent
                    className="w-22 bg-secondary dark:bg-zinc-800 shadow-2xl"
                    side="top"
                >
                    <div className="grid gap-4">
                        <Button
                            className="dark:bg-zinc-600 dark:text-white"
                            onClick={() => createNewStep("while")}
                        >
                            <RotateCw className="w-4 h-4 mr-2" /> While
                        </Button>
                        <Button
                            className="dark:bg-zinc-600 dark:text-white"
                            onClick={() => createNewStep("if")}
                        >
                            <GitFork className="w-4 h-4 mr-2" /> If
                        </Button>
                        <Button
                            className="dark:bg-zinc-600 dark:text-white"
                            onClick={() => createNewStep("regular")}
                        >
                            <ArrowRight className="w-4 h-4 mr-2" /> Step
                        </Button>
                    </div>
                </PopoverContent>
            </Popover>

            <Dialog
                open={isTemplateModalOpen}
                onOpenChange={setIsTemplateModalOpen}
            >
                <DialogContent className="min-w-[50vw] max-h-[80vh]">
                    <DialogHeader>
                        <DialogTitle>Select a Template</DialogTitle>
                    </DialogHeader>
                    <ScrollArea className="grid gap-4 h-[70vh]">
                        {stepTemplates.map((template, index) => (
                            <StepTemplateCard
                                key={index}
                                stepTemplate={template}
                                onClick={() =>
                                    handleStepTemplateSelection(template)
                                }
                            />
                        ))}
                    </ScrollArea>
                </DialogContent>
            </Dialog>
        </>
    );
}
