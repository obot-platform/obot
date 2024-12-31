import { If, Step } from "~/lib/model/workflows";

import { AddStepButton } from "~/components/workflow/steps/AddStep";

export function IfContent({
    ifCondition,
    onUpdate,
    renderStep,
}: {
    ifCondition: If;
    onUpdate: (updatedIf: If) => void;
    renderStep: (
        step: Step,
        onUpdate: (updatedStep: Step) => void,
        onDelete: () => void
    ) => React.ReactNode;
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
                {ifCondition.steps?.map((step, index) => (
                    <div key={index} className="ml-4">
                        {renderStep(
                            step,
                            (updatedStep) =>
                                updateNestedStep("steps", index, updatedStep),
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
                                updateNestedStep("else", index, updatedStep),
                            () => deleteNestedStep("else", index)
                        )}
                    </div>
                ))}
                <div className="ml-4">
                    <AddStepButton onAddStep={addStep("else")} />
                </div>
            </div>
        </div>
    );
}
