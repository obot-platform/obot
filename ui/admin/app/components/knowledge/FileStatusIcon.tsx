import {
	CheckIcon,
	FileClock,
	PlusIcon,
	RotateCcwIcon,
	ShieldAlert,
} from "lucide-react";

import { KnowledgeFile, KnowledgeFileState } from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

const fileStateIcons: Record<KnowledgeFileState, [React.ElementType, string]> =
	{
		[KnowledgeFileState.PendingApproval]: [PlusIcon, ""],
		[KnowledgeFileState.Pending]: [FileClock, ""],
		[KnowledgeFileState.Ingesting]: [LoadingSpinner, ""],
		[KnowledgeFileState.Ingested]: [CheckIcon, "text-green-500"],
		[KnowledgeFileState.Error]: [RotateCcwIcon, "text-destructive"],
		[KnowledgeFileState.Unapproved]: [PlusIcon, "text-warning"],
		[KnowledgeFileState.Unsupported]: [ShieldAlert, "text-warning"],
	} as const;

type FileStatusIconProps = {
	file: KnowledgeFile;
};

const FileStatusIcon: React.FC<FileStatusIconProps> = ({ file }) => {
	const [Icon, className] = fileStateIcons[file.state];

	return (
		<div className={cn("flex items-center", className)}>
			<Tooltip>
				<TooltipTrigger asChild>
					<div>
						{Icon === LoadingSpinner ? (
							<LoadingSpinner className={cn("h-4 w-4", className)} />
						) : (
							<Icon className={cn("h-4 w-4", className)} />
						)}
					</div>
				</TooltipTrigger>
				<TooltipContent className="max-h-full max-w-[300px] whitespace-normal break-words">
					{file.state.charAt(0).toUpperCase() +
						file.state.slice(1).toLowerCase()}
				</TooltipContent>
			</Tooltip>
		</div>
	);
};

export default FileStatusIcon;
