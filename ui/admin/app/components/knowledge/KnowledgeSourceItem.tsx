import { Edit, RefreshCcw, Trash } from "lucide-react";

import {
	KnowledgeSource,
	KnowledgeSourceStatus,
	getKnowledgeSourceDisplayName,
	getKnowledgeSourceType,
} from "~/lib/model/knowledge";

import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "~/components/ui/tooltip";

interface KnowledgeSourceItemProps {
	source: KnowledgeSource;
	onSync: (sourceId: string) => void;
	onEdit: (sourceId: string) => void;
	onDelete: (sourceId: string) => void;
}

export function KnowledgeSourceItem({
	source,
	onSync,
	onEdit,
	onDelete,
}: KnowledgeSourceItemProps) {
	const isSyncing =
		source.state === KnowledgeSourceStatus.Syncing ||
		source.state === KnowledgeSourceStatus.Pending;

	return (
		<div className="flex w-full items-center justify-between rounded-md border px-2">
			<div className="flex items-center">
				<KnowledgeSourceAvatar
					knowledgeSourceType={getKnowledgeSourceType(source)}
					className="h-4 w-4"
				/>
				<span>{getKnowledgeSourceDisplayName(source)}</span>
			</div>
			<div className="flex items-center">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							onClick={() => onSync(source.id)}
							disabled={isSyncing}
						>
							{isSyncing ? (
								<LoadingSpinner className="h-4 w-4" />
							) : (
								<RefreshCcw className="h-4 w-4" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						{isSyncing ? (source.status ?? "Syncing...") : "Sync"}
					</TooltipContent>
				</Tooltip>

				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="ghost"
							size="icon"
							onClick={() => onEdit(source.id)}
						>
							<Edit className="h-4 w-4" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>Edit</TooltipContent>
				</Tooltip>

				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								onClick={() => onDelete(source.id)}
							>
								<Trash className="h-4 w-4" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>Delete</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</div>
		</div>
	);
}
