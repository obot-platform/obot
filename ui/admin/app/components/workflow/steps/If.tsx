import { ChevronDown, ChevronRight, GitFork, Trash } from "lucide-react";
import { useState } from "react";

import { If, Step } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { Textarea } from "~/components/ui/textarea";

import { AddStepButton } from "./AddStep";

export function IfComponent({
    ifCondition,
    onUpdate,
    onDelete,
    renderStep,
    className,
}: {
    ifCondition: If;
    onUpdate: (updatedIf: If) => void;
    onDelete: () => void;
    renderStep: (
        step: Step,
        onUpdate: (updatedStep: Step) => void,
        onDelete: () => void
    ) => React.ReactNode;
    className?: string;
}) {
    const [isExpanded, setIsExpanded] = useState(false);

    const addStep = (branch: "steps" | "else") => (newStep: Step) => {
        onUpdate({
            ...ifCondition,
            [branch]: [...(ifCondition[branch] || []), newStep],
        });
    };

    const updateNestedStep = (
        branch: "steps" | "else",
        index: number,
        updatedStep: Step
    ) => {
        const newBranch = [...(ifCondition[branch] || [])];
        newBranch[index] = updatedStep;
        onUpdate({ ...ifCondition, [branch]: newBranch });
    };

    const deleteNestedStep = (branch: "steps" | "else", index: number) => {
        const newBranch = (ifCondition[branch] || []).filter(
            (_, i) => i !== index
        );
        onUpdate({ ...ifCondition, [branch]: newBranch });
    };

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
                    <GitFork className="w-4 h-4 mr-1" />
                    <span className="text-sm font-medium">If</span>
                </div>
                <Textarea
                    value={ifCondition.condition || ""}
                    onChange={(e) =>
                        onUpdate({ ...ifCondition, condition: e.target.value })
                    }
                    placeholder="Condition"
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
                <div className="p-3 space-y-4">
                    <div className="space-y-2">
                        <h4 className="font-semibold">Then:</h4>
                        {ifCondition.steps?.map((step, index) => (
                            <div key={index} className="ml-4">
                                {renderStep(
                                    step,
                                    (updatedStep) =>
                                        updateNestedStep(
                                            "steps",
                                            index,
                                            updatedStep
                                        ),
                                    () => deleteNestedStep("steps", index)
                                )}
                            </div>
                        ))}
                        <div className="ml-4">
                            <AddStepButton onAddStep={addStep("steps")} />
                        </div>
                    </div>
                    <div className="space-y-2">
                        <h4 className="font-semibold">Else:</h4>
                        {ifCondition.else?.map((step, index) => (
                            <div key={index} className="ml-4">
                                {renderStep(
                                    step,
                                    (updatedStep) =>
                                        updateNestedStep(
                                            "else",
                                            index,
                                            updatedStep
                                        ),
                                    () => deleteNestedStep("else", index)
                                )}
                            </div>
                        ))}
                        <div className="ml-4">
                            <AddStepButton onAddStep={addStep("else")} />
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
