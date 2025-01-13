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
	clamp = true,
}: {
	children: React.ReactNode;
	className?: string;
	asChild?: boolean;
	disableTooltip?: boolean;
	tooltipContent?: React.ReactNode;
	clamp?: boolean;
}) {
	const Comp = asChild ? Slot : "p";

	const content = (
		<Comp className={cn({ "line-clamp-1": clamp, truncate: !clamp })}>
			{children}
		</Comp>
	);

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
