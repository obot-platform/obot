import { Step } from "~/lib/model/workflows";

import { IfComponent } from "~/components/workflow/steps/If";
import { WhileComponent } from "~/components/workflow/steps/While";

import { StepComponent } from "./Step";
import { TemplateComponent } from "./Template";

export function renderStep(
    step: Step,
    onUpdate: (updatedStep: Step) => void,
    onDelete: () => void
) {
    if (step.template) {
        return (
            <TemplateComponent
                key={step.id}
                step={step}
                onUpdate={onUpdate}
                onDelete={onDelete}
            />
        );
    } else if (step.if) {
        return (
            <IfComponent
                key={step.id}
                ifCondition={step.if}
                onUpdate={(updatedIf) => onUpdate({ ...step, if: updatedIf })}
                onDelete={onDelete}
                renderStep={renderStep}
                className="mb-4"
            />
        );
    } else if (step.while) {
        return (
            <WhileComponent
                key={step.id}
                whileCondition={step.while}
                onUpdate={(updatedWhile) =>
                    onUpdate({ ...step, while: updatedWhile })
                }
                onDelete={onDelete}
                renderStep={renderStep}
                className="mb-4"
            />
        );
    } else {
        return (
            <StepComponent
                key={step.id}
                step={step}
                onUpdate={onUpdate}
                onDelete={onDelete}
                className="mb-4"
            />
        );
    }
}
