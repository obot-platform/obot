import { EyeIcon, FileIcon, RotateCcwIcon, Trash } from "lucide-react";

import { KnowledgeFile, KnowledgeFileState } from "~/lib/model/knowledge";

import FileStatusIcon from "~/components/knowledge/FileStatusIcon";
import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "~/components/ui/tooltip";

interface KnowledgeFileItemProps {
	file: KnowledgeFile;
	onDelete: (file: KnowledgeFile) => void;
	onReingest: (file: KnowledgeFile) => void;
	onViewError?: (error: string) => void;
}

export function KnowledgeFileItem({
	file,
	onDelete,
	onReingest,
	onViewError,
}: KnowledgeFileItemProps) {
	const formatFileSize = (bytes: number) => {
		if (bytes > 1000000) {
			return (bytes / 1000000).toFixed(2) + " MB";
		}
		return (bytes / 1000).toFixed(2) + " KB";
	};

	return (
		<div className="flex w-full items-center justify-between rounded-md border px-2">
			<div className="flex items-center">
				<FileIcon className="mr-2 h-4 w-4" />
				<span>{file.fileName}</span>
			</div>
			<div className="flex items-center">
				<div className="mr-2 text-xs text-gray-400">
					{file.sizeInBytes ? formatFileSize(file.sizeInBytes) : "0 Bytes"}
				</div>
				<div>
					{file.state === KnowledgeFileState.Error ? (
						<div className="flex items-center">
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										onClick={() => onReingest(file)}
									>
										<RotateCcwIcon className="h-4 w-4 text-destructive" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>Reingest</TooltipContent>
							</Tooltip>

							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										onClick={() => onViewError?.(file.error ?? "")}
									>
										<EyeIcon className="h-4 w-4 text-destructive" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>View Error</TooltipContent>
							</Tooltip>
						</div>
					) : (
						<div className="mr-2 flex items-center">
							<FileStatusIcon file={file} />
						</div>
					)}
				</div>
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								onClick={() => onDelete(file)}
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
