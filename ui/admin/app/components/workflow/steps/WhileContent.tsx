import { Step, While } from "~/lib/model/workflows";

import { Input } from "~/components/ui/input";
import { AddStepButton } from "~/components/workflow/steps/AddStep";

export function WhileContent({
    whileCondition,
    onUpdate,
    renderStep,
}: {
    whileCondition: While;
    onUpdate: (updatedWhile: While) => void;
    renderStep: (
        step: Step,
        onUpdate: (updatedStep: Step) => void,
        onDelete: () => void
    ) => React.ReactNode;
    className?: string;
}) {
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
    );
}
