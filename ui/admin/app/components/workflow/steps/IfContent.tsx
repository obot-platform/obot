import { If, Step } from "~/lib/model/workflows";

import { SortableList } from "~/components/ui/dnd/sortable";
import { AddStepButton } from "~/components/workflow/steps/AddStep";
import type { StepRenderer } from "~/components/workflow/steps/step-renderer-helpers";

export function IfContent({
    ifCondition,
    onUpdate,
    renderStep,
}: {
    ifCondition: If;
    onUpdate: (updatedIf: If) => void;
    renderStep: StepRenderer;
    className?: string;
}) {
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
        <div className="p-3 space-y-4">
            <div className="space-y-2">
                <h4 className="font-semibold">Then:</h4>

                <SortableList
                    items={ifCondition.steps || []}
                    renderItem={(step, index) =>
                        renderStep({
                            step,
                            onDelete: () => deleteNestedStep("steps", index),
                            onUpdate: (updatedStep) =>
                                updateNestedStep("steps", index, updatedStep),
                        })
                    }
                    getKey={(step) => step.id}
                    onChange={(newSteps) =>
                        onUpdate({ ...ifCondition, steps: newSteps })
                    }
                    isHandle={false}
                />

                <div className="ml-4">
                    <AddStepButton onAddStep={addStep("steps")} />
                </div>
            </div>
            <div className="space-y-2">
                <h4 className="font-semibold">Else:</h4>
                <SortableList
                    items={ifCondition.else || []}
                    renderItem={(step, index) =>
                        renderStep({
                            step,
                            onDelete: () => deleteNestedStep("else", index),
                            onUpdate: (updatedStep) =>
                                updateNestedStep("else", index, updatedStep),
                        })
                    }
                    getKey={(step) => step.id}
                    onChange={(newSteps) =>
                        onUpdate({ ...ifCondition, else: newSteps })
                    }
                    isHandle={false}
                />

                <div className="ml-4">
                    <AddStepButton onAddStep={addStep("else")} />
                </div>
            </div>
        </div>
    );
}
