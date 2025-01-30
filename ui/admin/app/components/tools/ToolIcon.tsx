import { WrenchIcon } from "lucide-react";

import { cn } from "~/lib/utils";

type ToolIconProps = {
	name: string;
	category?: string;
	icon?: string;
	className?: string;
};

export function ToolIcon(props: ToolIconProps) {
	const { name, icon, className } = props;

	return icon ? (
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
}
