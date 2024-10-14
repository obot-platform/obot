import { ChevronDown, ChevronRight, Trash } from "lucide-react";
import { useState } from "react";

import { Step, Template } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { Textarea } from "~/components/ui/textarea";

export function TemplateComponent({
    step,
    onUpdate,
    onDelete,
    className,
}: {
    step: Step;
    onUpdate: (updatedStep: Step) => void;
    onDelete: () => void;
    className?: string;
}) {
    const [isExpanded, setIsExpanded] = useState(false);

    if (!step.template) {
        console.error("TemplateComponent received a step without a template");
        return null;
    }

    const renderTemplateArgs = (template: Template) => {
        return Object.entries(template.args).map(([key, value]) => {
            return (
                <div key={key} className="mb-4">
                    <label
                        htmlFor={key}
                        className="block text-sm font-medium mb-1"
                    >
                        {key}
                    </label>
                    <Textarea
                        id={key}
                        placeholder={value}
                        value={value}
                        onChange={(e) => {
                            const updatedTemplate = {
                                ...template,
                                args: {
                                    ...template.args,
                                    [key]: e.target.value,
                                },
                            };
                            onUpdate({ ...step, template: updatedTemplate });
                        }}
                        className="bg-background"
                    />
                </div>
            );
        });
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
                <div className="flex-grow bg-background p-2 rounded-md border">
                    {step.name}
                </div>
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
                <div className="p-3 space-y-4 px-8">
                    <div className="mb-4">
                        {renderTemplateArgs(step.template)}
                    </div>
                </div>
            )}
        </div>
    );
}
