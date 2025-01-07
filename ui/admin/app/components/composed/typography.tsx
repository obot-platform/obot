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
    tooltipContent = <p>{children}</p>,
}: {
    children: React.ReactNode;
    className?: string;
    asChild?: boolean;
    disableTooltip?: boolean;
    tooltipContent?: React.ReactNode;
}) {
    const Comp = asChild ? Slot : "p";

    if (disableTooltip) {
        return <Comp className="truncate">{children}</Comp>;
    }

    return (
        <Tooltip>
            <TooltipContent>{tooltipContent}</TooltipContent>

            <TooltipTrigger asChild>
                <div className={cn("cursor-pointer", className)}>
                    <Comp className="truncate">{children}</Comp>
                </div>
            </TooltipTrigger>
        </Tooltip>
    );
}
