import { ChevronDown, ChevronRight, Trash } from "lucide-react";
import { useState } from "react";

import { Step, StepType, getDefaultStep } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { StepTypeSelect } from "~/components/workflow/steps/StepTypeSelect";

export function StepBase({
    className,
    children,
    step,
    type,
    onUpdate,
    onDelete,
}: {
    className?: string;
    children: React.ReactNode;
    step: Step;
    type: StepType;
    onUpdate: (step: Step) => void;
    onDelete: () => void;
}) {
    const [isExpanded, setIsExpanded] = useState(false);

    const fieldConfig = getTextFieldConfig();

    return (
        <div className={cn("border rounded-md", className)}>
            <ClickableDiv
                className={cn(
                    "flex items-start gap-2 p-3 bg-muted",
                    isExpanded ? "rounded-t-md" : "rounded-md"
                )}
                onClick={() => setIsExpanded((prev) => !prev)}
            >
                <div className="flex items-center gap-2">
                    <Button
                        variant="ghost"
                        size="icon"
                        className="p-0 w-6 h-6 self-center"
                    >
                        {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                        ) : (
                            <ChevronRight className="w-4 h-4" />
                        )}
                    </Button>

                    <StepTypeSelect
                        value={type}
                        onChange={(newType) => {
                            if (newType !== type) {
                                onUpdate(getDefaultStep(newType));
                            }
                        }}
                    />
                </div>

                <AutosizeTextarea
                    value={fieldConfig.value}
                    onChange={(e) => fieldConfig.onChange(e.target.value)}
                    placeholder={fieldConfig.placeholder}
                    maxHeight={100}
                    minHeight={0}
                    className="flex-grow bg-background-secondary"
                    onClick={(e) => e.stopPropagation()}
                />

                <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => {
                        e.stopPropagation();
                        onDelete();
                    }}
                >
                    <Trash className="w-4 h-4" />
                </Button>
            </ClickableDiv>

            {isExpanded && children}
        </div>
    );

    function getTextFieldConfig() {
        if (type === "if" && step.if) {
            const copy = step.if;

            return {
                placeholder: "Condition",
                value: step.if.condition,
                onChange: (value: string) =>
                    onUpdate({ ...step, if: { ...copy, condition: value } }),
            };
        } else if (type === "while" && step.while) {
            const copy = step.while;

            return {
                placeholder: "Condition",
                value: step.while.condition,
                onChange: (value: string) =>
                    onUpdate({ ...step, while: { ...copy, condition: value } }),
            };
        }

        return {
            placeholder: "Step",
            value: step.step,
            onChange: (value: string) => onUpdate({ ...step, step: value }),
        };
    }
}
