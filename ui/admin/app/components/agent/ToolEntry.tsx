import { TrashIcon } from "lucide-react";
import { useMemo } from "react";
import useSWR from "swr";

import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { Truncate } from "~/components/composed/typography";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";

export function ToolEntry({
	tool,
	onDelete,
	actions,
	withDescription = false,
}: {
	tool: string;
	onDelete?: () => void;
	actions?: React.ReactNode;
	withDescription?: boolean;
}) {
	const toolInfo = useToolReference(tool);
	const description = toolInfo.toolReference?.description;

	return (
		<div className="flex flex-col">
			<div className="mt-1 flex items-center justify-between space-x-2">
				<div className="flex w-full items-center justify-between gap-2 rounded-md p-2 px-3 text-sm">
					<div className="flex items-center gap-2">
						{toolInfo.icon}

						<div className="flex flex-col">
							<Truncate
								classNames={{ content: "font-medium" }}
								tooltipContent={withDescription ? toolInfo.label : description}
								tooltipContentProps={{ align: "start", className: "max-w-xs" }}
							>
								{toolInfo.label}
							</Truncate>

							{withDescription && description && (
								<Truncate tooltipContent={description} asChild>
									<small className="text-muted-foreground">{description}</small>
								</Truncate>
							)}
						</div>
					</div>

					<div className="flex items-center gap-2">
						{actions}

						{onDelete && (
							<Button
								type="button"
								variant="ghost"
								size="icon"
								onClick={() => onDelete()}
							>
								<TrashIcon className="h-5 w-5" />
							</Button>
						)}
					</div>
				</div>
			</div>
		</div>
	);
}

export function useToolReference(tool: string) {
	const { data: toolReference, isLoading } = useSWR(
		ToolReferenceService.getToolReferenceById.key(tool),
		({ toolReferenceId }) =>
			ToolReferenceService.getToolReferenceById(toolReferenceId),
		{ errorRetryCount: 0, revalidateIfStale: false }
	);

	const icon = useMemo(
		() =>
			isLoading ? (
				<LoadingSpinner className="h-6 w-6 min-w-fit" />
			) : (
				<ToolIcon
					className="h-6 w-6 min-w-fit"
					name={toolReference?.name || tool}
					icon={toolReference?.metadata?.icon}
				/>
			),
		[isLoading, toolReference, tool]
	);

	const label = toolReference?.name || tool;

	return { toolReference, isLoading, icon, label };
}
