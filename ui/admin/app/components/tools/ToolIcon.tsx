import { WrenchIcon } from "lucide-react";

import { cn } from "~/lib/utils";

import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type ToolIconProps = {
	name: string;
	category?: string;
	icon?: string;
	className?: string;
	disableTooltip?: boolean;
};

export function ToolIcon(props: ToolIconProps) {
	const { name, category, icon, className, disableTooltip } = props;

	const content = icon ? (
		<img
			alt={name}
			src={icon}
			className={cn("h-6 w-6", className, {
				// icons served from /admin/assets are colored, so we should not invert them.
				"dark:invert": !icon.startsWith("/admin/assets"),
			})}
		/>
	) : (
		<WrenchIcon className={cn("mr-2 h-4 w-4", className)} />
	);

	if (disableTooltip) {
		return content;
	}

	return (
		<Tooltip>
			<TooltipTrigger>{content}</TooltipTrigger>

			<TooltipContent>
				{[category, name].filter((x) => !!x).join(" - ")}
			</TooltipContent>
		</Tooltip>
	);
}
