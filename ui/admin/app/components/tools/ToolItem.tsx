import { TriangleAlertIcon } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolTooltip } from "~/components/tools/ToolTooltip";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { CommandItem } from "~/components/ui/command";

type ToolItemProps = {
	tool: ToolReference;
	configured: boolean;
	isSelected: boolean;
	isBundleSelected: boolean;
	onSelect: () => void;
	expanded?: boolean;
	onExpand?: (expanded: boolean) => void;
	className?: string;
	isBundle?: boolean;
};

export function ToolItem({
	tool,
	configured,
	isSelected,
	isBundleSelected,
	onSelect,
	expanded,
	onExpand,
	className,
	isBundle,
}: ToolItemProps) {
	return (
		<CommandItem
			className={cn("cursor-pointer", className)}
			onSelect={configured ? onSelect : undefined}
			disabled={isBundleSelected}
		>
			<ToolTooltip tool={tool} requiresConfiguration={!configured}>
				<div className={cn("flex w-full items-center justify-between gap-2")}>
					<span
						className={cn(
							"flex w-full items-center gap-2 px-4 text-sm font-medium",
							{
								"px-0": isBundle,
							}
						)}
					>
						{configured ? (
							<Checkbox checked={isSelected || isBundleSelected} />
						) : isBundle ? (
							<TriangleAlertIcon className="h-4 w-4 text-warning opacity-50" />
						) : null}

						<span
							className={cn("flex items-center", !configured && "opacity-50")}
						>
							<ToolIcon
								icon={tool.metadata?.icon}
								category={tool.metadata?.category}
								name={tool.name}
								className="mr-2 h-4 w-4"
								disableTooltip
							/>
							{tool.name}
						</span>
					</span>

					{isBundle && (
						<Button
							variant="link"
							size="link-sm"
							onClick={(e) => {
								e.stopPropagation();
								onExpand?.(!expanded);
							}}
						>
							{expanded ? "Show Less" : "Show More"}
						</Button>
					)}
				</div>
			</ToolTooltip>
		</CommandItem>
	);
}
