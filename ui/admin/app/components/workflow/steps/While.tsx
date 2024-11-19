import { ChevronDown, ChevronRight, RotateCw, Trash } from "lucide-react";
import { useState } from "react";

import { Step, While } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { Input } from "~/components/ui/input";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { AddStepButton } from "~/components/workflow/steps/AddStep";

export function WhileComponent({
    whileCondition,
    onUpdate,
    onDelete,
    renderStep,
    className,
}: {
    whileCondition: While;
    onUpdate: (updatedWhile: While) => void;
    onDelete: () => void;
    renderStep: (
        step: Step,
        onUpdate: (updatedStep: Step) => void,
        onDelete: () => void
    ) => React.ReactNode;
    className?: string;
}) {
    const [isExpanded, setIsExpanded] = useState(false);

    const addStep = (newStep: Step) => {
        onUpdate({
            ...whileCondition,
            steps: [...(whileCondition.steps || []), newStep],
        });
    };

    const updateNestedStep = (index: number, updatedStep: Step) => {
        const newSteps = [...(whileCondition.steps || [])];
        newSteps[index] = updatedStep;
        onUpdate({ ...whileCondition, steps: newSteps });
    };

    const deleteNestedStep = (index: number) => {
        const newSteps = (whileCondition.steps || []).filter(
            (_, i) => i !== index
        );
        onUpdate({ ...whileCondition, steps: newSteps });
    };

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
                        onClick={() => setIsExpanded(!isExpanded)}
                    >
                        {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                        ) : (
                            <ChevronRight className="w-4 h-4" />
                        )}
                    </Button>

                    <div className="flex items-center justify-center w-24 p-2 border bg-background rounded-md mr-2">
                        <RotateCw className="w-4 h-4 mr-1" />
                        <span className="text-sm font-medium">While</span>
                    </div>
                </div>

                <AutosizeTextarea
                    value={whileCondition.condition}
                    onChange={(e) =>
                        onUpdate({
                            ...whileCondition,
                            condition: e.target.value,
                        })
                    }
                    minHeight={0}
                    maxHeight={100}
                    placeholder="Condition"
                    className="flex-grow bg-background"
                    onClick={(e) => e.stopPropagation()}
                />
                <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => {
                        e.stopPropagation();
                        onDelete();
                    }}
                    className="ml-2"
                >
                    <Trash className="w-4 h-4" />
                </Button>
            </ClickableDiv>

            {isExpanded && (
                <div className="p-3 space-y-4">
                    <div>
                        <label
                            htmlFor="maxLoops"
                            className="block text-sm font-medium text-gray-700 mb-1"
                        >
                            Max Loops
                        </label>
                        <Input
                            id="maxLoops"
                            type="number"
                            value={whileCondition.maxLoops}
                            onChange={(e) =>
                                onUpdate({
                                    ...whileCondition,
                                    maxLoops: parseInt(e.target.value),
                                })
                            }
                            placeholder="Max Loops"
                            className="bg-background"
                        />
                    </div>

                    <div className="space-y-2">
                        <h4 className="font-semibold">Steps:</h4>
                        {whileCondition.steps?.map((step, index) => (
                            <div key={index} className="ml-4">
                                {renderStep(
                                    step,
                                    (updatedStep) =>
                                        updateNestedStep(index, updatedStep),
                                    () => deleteNestedStep(index)
                                )}
                            </div>
                        ))}

                        <div className="ml-4">
                            <AddStepButton onAddStep={addStep} />
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
