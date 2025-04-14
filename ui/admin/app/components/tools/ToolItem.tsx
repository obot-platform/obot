import { TriangleAlertIcon } from "lucide-react";
import { useState } from "react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils";

import { ToolOauthConfig } from "~/components/tools//ToolOauthConfig";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolTooltip } from "~/components/tools/ToolTooltip";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { CommandItem } from "~/components/ui/command";

type ToolItemProps = {
	tool: ToolReference;
	configured: boolean;
	hideWarning?: boolean;
	isSelected: boolean;
	onSelect: (oAuthToAdd?: string) => void;
	expanded?: boolean;
	onExpand?: (expanded: boolean) => void;
	className?: string;
	isGroup?: boolean;
	isBundle?: boolean;
};

export function ToolItem({
	tool,
	configured,
	hideWarning,
	isSelected,
	onSelect,
	expanded,
	onExpand,
	className,
	isGroup,
	isBundle,
}: ToolItemProps) {
	const [toolOAuthDialogOpen, setToolOAuthDialogOpen] = useState(false);

	const isPATSupported = tool.metadata?.supportsOAuthTokenPrompt === "true";
	const oAuthMetadata = tool.metadata?.oauth;
	const available = configured || isPATSupported;

	const handleSelect = () => {
		onSelect(configured && oAuthMetadata ? oAuthMetadata : undefined);
	};

	return (
		<>
			<CommandItem
				className={cn("cursor-pointer", className)}
				onSelect={available ? handleSelect : undefined}
			>
				<ToolTooltip
					tool={tool}
					requiresConfiguration={!available}
					onConfigureAuth={() => setToolOAuthDialogOpen(true)}
				>
					<div className={cn("flex w-full items-center justify-between gap-2")}>
						<span
							className={cn(
								"flex w-full items-center gap-2 px-4 text-sm font-medium",
								{ "px-0": isGroup }
							)}
						>
							{available ? (
								<Checkbox checked={isSelected} />
							) : !hideWarning ? (
								<TriangleAlertIcon className="h-4 w-4 text-warning opacity-50" />
							) : null}

							<span
								className={cn("flex items-center", !available && "opacity-50")}
							>
								<ToolIcon
									icon={tool.metadata?.icon}
									category={tool.metadata?.category}
									name={tool.name || normalizeToolID(tool.id)}
									className="mr-2 h-4 w-4"
								/>
								{tool.name || normalizeToolID(tool.id)}
								{isBundle && <Badge className="ms-2">Bundle</Badge>}
							</span>
						</span>

						{isGroup && (
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
			{oAuthMetadata && !isPATSupported && (
				<ToolOauthConfig
					tool={tool}
					open={toolOAuthDialogOpen}
					onOpenChange={setToolOAuthDialogOpen}
					onSuccess={() => setToolOAuthDialogOpen(false)}
				/>
			)}
		</>
	);
}

function normalizeToolID(toolId: string) {
	return toolId.replace(/-/g, " ");
}
