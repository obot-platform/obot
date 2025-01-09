import { Slot } from "@radix-ui/react-slot";

import { cn } from "~/lib/utils";

import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function Truncate({
    children,
    className,
    asChild,
    disableTooltip,
    tooltipContent = children,
}: {
    children: React.ReactNode;
    className?: string;
    asChild?: boolean;
    disableTooltip?: boolean;
    tooltipContent?: React.ReactNode;
}) {
    const Comp = asChild ? Slot : "p";

    const content = <Comp className="truncate">{children}</Comp>;

    if (disableTooltip) {
        return content;
    }

    return (
        <Tooltip>
            <TooltipContent>{tooltipContent}</TooltipContent>

            <TooltipTrigger asChild>
                <div className={cn("cursor-pointer", className)}>{content}</div>
            </TooltipTrigger>
        </Tooltip>
    );
}
