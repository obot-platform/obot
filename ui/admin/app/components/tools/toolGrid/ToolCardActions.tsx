import { EllipsisVerticalIcon } from "lucide-react";
import { toast } from "sonner";

import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { useAsync } from "~/hooks/useAsync";
import { usePollSingleTool } from "~/hooks/usePollSingleTool";

export function ToolCardActions({ tool }: { tool: ToolReference }) {
	const { startPolling, isPolling } = usePollSingleTool(tool.id);

	const forceRefresh = useAsync(
		ToolReferenceService.forceRefreshToolReference,
		{
			onSuccess: () => {
				toast.success("Tool reference force refreshed");
				startPolling();
			},
		}
	);

	return (
		<div className="flex items-center gap-2">
			{(forceRefresh.isLoading || isPolling) && <LoadingSpinner />}

			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="ghost" size="icon" className="m-0">
						<EllipsisVerticalIcon />
					</Button>
				</DropdownMenuTrigger>

				<DropdownMenuContent side="top" align="start">
					<DropdownMenuItem onClick={() => forceRefresh.execute(tool.id)}>
						Refresh Tool
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);
}
