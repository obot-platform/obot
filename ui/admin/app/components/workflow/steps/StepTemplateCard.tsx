import { PlusCircle } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";

import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function StepTemplateCard({
    stepTemplate,
    onClick,
}: {
    stepTemplate: ToolReference;
    onClick: () => void;
}) {
    return (
        <Card className="flex items-center justify-between truncate space-x-4 p-4 my-2">
            <div className="truncate text-sm">
                <h1 className="truncate">{stepTemplate.name}</h1>
                <h2 className="text-gray-500 truncate">
                    {stepTemplate.description}
                </h2>
            </div>
            <TooltipProvider>
                <Tooltip>
                    <TooltipContent>Add template</TooltipContent>
                    <TooltipTrigger>
                        <Button
                            onClick={onClick}
                            size="icon"
                            variant="secondary"
                        >
                            <PlusCircle />
                        </Button>
                    </TooltipTrigger>
                </Tooltip>
            </TooltipProvider>
        </Card>
    );
}
