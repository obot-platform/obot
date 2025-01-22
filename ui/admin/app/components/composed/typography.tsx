import { Slot } from "@radix-ui/react-slot";
import { TooltipContentProps } from "@radix-ui/react-tooltip";

import { cn } from "~/lib/utils";

import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export function Truncate({
	children,
	className,
	classNames,
	asChild,
	disableTooltip,
	tooltipContent = children,
	clamp = true,
	clampLength = 1,
	tooltipContentProps,
}: {
	children: React.ReactNode;
	className?: string;
	asChild?: boolean;
	disableTooltip?: boolean;
	tooltipContent?: React.ReactNode;
	clamp?: boolean;
	clampLength?: 1 | 2;
	classNames?: { content?: string };
	tooltipContentProps?: TooltipContentProps;
}) {
	const Comp = asChild ? Slot : "p";

	const content = (
		<Comp
			className={cn(
				{
					"line-clamp-1": clamp && clampLength === 1,
					"line-clamp-2": clamp && clampLength === 2,
					truncate: !clamp,
				},
				classNames?.content
			)}
		>
			{children}
		</Comp>
	);

	if (disableTooltip) {
		return content;
	}

	return (
		<Tooltip>
			<TooltipContent
				align="start"
				{...tooltipContentProps}
				className={cn("max-w-xs", tooltipContentProps?.className)}
			>
				{tooltipContent}
			</TooltipContent>

			<TooltipTrigger asChild>
				<div className={cn("cursor-pointer", className)}>{content}</div>
			</TooltipTrigger>
		</Tooltip>
	);
}
