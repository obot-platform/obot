import { Step } from "~/lib/model/workflows";

import { IfContent } from "~/components/workflow/steps/IfContent";
import { StepBase } from "~/components/workflow/steps/StepBase";
import { StepContent } from "~/components/workflow/steps/StepContent";
import { TemplateComponent } from "~/components/workflow/steps/Template";
import { WhileContent } from "~/components/workflow/steps/WhileContent";

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
            <StepBase
                key={step.id}
                step={step}
                type="if"
                onUpdate={onUpdate}
                onDelete={onDelete}
            >
                <IfContent
                    ifCondition={step.if}
                    onUpdate={(updatedIf) =>
                        onUpdate({ ...step, if: updatedIf })
                    }
                    renderStep={renderStep}
                />
            </StepBase>
        );
    } else if (step.while) {
        return (
            <StepBase
                key={step.id}
                step={step}
                type="while"
                onUpdate={onUpdate}
                onDelete={onDelete}
            >
                <WhileContent
                    whileCondition={step.while}
                    onUpdate={(updatedWhile) =>
                        onUpdate({ ...step, while: updatedWhile })
                    }
                    renderStep={renderStep}
                />
            </StepBase>
        );
    } else {
        return (
            <StepBase
                key={step.id}
                step={step}
                type="command"
                onUpdate={onUpdate}
                onDelete={onDelete}
            >
                <StepContent key={step.id} step={step} onUpdate={onUpdate} />
            </StepBase>
        );
    }
}
