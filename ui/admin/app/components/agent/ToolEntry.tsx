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
}: {
	tool: string;
	onDelete?: () => void;
	actions?: React.ReactNode;
}) {
	const toolInfo = useToolReference(tool);

	return (
		<div className="mt-1 flex items-center justify-between space-x-2">
			<div className="flex w-full items-center justify-between gap-2 rounded-md p-2 px-3 text-sm">
				<div className="flex items-center gap-2">
					{toolInfo.icon}

					<Truncate>{toolInfo.label}</Truncate>
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
				<LoadingSpinner className="h-5 w-5" />
			) : (
				<ToolIcon
					className="h-5 w-5"
					name={toolReference?.name || tool}
					icon={toolReference?.metadata?.icon}
				/>
			),
		[isLoading, toolReference, tool]
	);

	const label = toolReference?.name || tool;

	return { toolReference, isLoading, icon, label };
}
