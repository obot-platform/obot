import { TriangleAlertIcon } from "lucide-react";
import { useState } from "react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils";

import { SelectToolAuth } from "~/components/tools/SelectToolAuth";
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
	onSelect: (oAuthToAdd?: string) => void;
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
	const [toolOAuthDialogOpen, setToolOAuthDialogOpen] = useState(false);

	const isPATSupported = tool.metadata?.supportsOAuthTokenPrompt === "true";
	const oAuthMetadata = tool.metadata?.oauth;
	const available = configured || isPATSupported;

	const handleSelect = () => {
		if (oAuthMetadata && isPATSupported && !isSelected) {
			setToolOAuthDialogOpen(true);
		} else {
			onSelect(configured && oAuthMetadata ? oAuthMetadata : undefined);
		}
	};

	const handleOAuthSelect = () => {
		if (!oAuthMetadata) return;
		setToolOAuthDialogOpen(false);
		onSelect(tool.metadata!.oauth);
	};

	const handlePATSelect = () => {
		setToolOAuthDialogOpen(false);
		onSelect();
	};

	return (
		<>
			<CommandItem
				className={cn("cursor-pointer", className)}
				onSelect={available ? handleSelect : undefined}
				disabled={isBundleSelected}
			>
				<ToolTooltip tool={tool} requiresConfiguration={!available}>
					<div className={cn("flex w-full items-center justify-between gap-2")}>
						<span
							className={cn(
								"flex w-full items-center gap-2 px-4 text-sm font-medium",
								{
									"px-0": isBundle,
								}
							)}
						>
							{available ? (
								<Checkbox checked={isSelected || isBundleSelected} />
							) : isBundle ? (
								<TriangleAlertIcon className="h-4 w-4 text-warning opacity-50" />
							) : null}

							<span
								className={cn("flex items-center", !available && "opacity-50")}
							>
								<ToolIcon
									icon={tool.metadata?.icon}
									category={tool.metadata?.category}
									name={tool.name}
									className="mr-2 h-4 w-4"
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
			{oAuthMetadata && isPATSupported && (
				<SelectToolAuth
					integration={oAuthMetadata}
					configured={configured}
					open={toolOAuthDialogOpen}
					onOpenChange={setToolOAuthDialogOpen}
					onOAuthSelect={handleOAuthSelect}
					onPATSelect={handlePATSelect}
				/>
			)}
		</>
	);
}
