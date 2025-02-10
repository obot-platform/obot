import { TriangleAlertIcon, WrenchIcon } from "lucide-react";
import { $path } from "safe-routes";

import { ToolReference } from "~/lib/model/toolReferences";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { Link } from "~/components/ui/link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type ToolTooltipProps = {
	tool: ToolReference;
	children: React.ReactNode;
	requiresConfiguration?: boolean;
	isBundle?: boolean;
};

export function ToolTooltip({
	tool,
	children,
	requiresConfiguration,
	isBundle = false,
}: ToolTooltipProps) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>{children}</TooltipTrigger>
			<TooltipContent
				sideOffset={isBundle ? 255 : 30}
				side={isBundle ? "left" : "left"}
				className="flex w-[300px] items-center border bg-background p-4 text-foreground"
			>
				{tool.metadata?.icon ? (
					<ToolIcon
						icon={tool.metadata?.icon}
						category={tool.metadata?.category}
						name={tool.name}
						className="mr-4 h-10 w-10"
					/>
				) : (
					<WrenchIcon className="mr-2 h-4 w-4" />
				)}
				<div>
					<p className="font-bold">
						{tool.name}
						{isBundle ? " Bundle" : ""}
					</p>
					<p className="text-sm">
						{tool.description || "No description provided."}
					</p>
					{requiresConfiguration && (
						<>
							<div className="flex items-center gap-1 pt-2 text-xs text-warning">
								<span>
									<TriangleAlertIcon className="h-4 w-4 text-warning" />
								</span>
								<p>
									<Link to={$path("/tools")} className="text-xs">
										Setup
									</Link>{" "}
									required to use this tool.
								</p>
							</div>
						</>
					)}
				</div>
			</TooltipContent>
		</Tooltip>
	);
}
