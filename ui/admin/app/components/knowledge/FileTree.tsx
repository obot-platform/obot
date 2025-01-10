import {
	CheckIcon,
	ChevronDown,
	ChevronRight,
	CircleX,
	Eye,
	File,
	FileClock,
	Folder,
	FolderOpen,
	MinusIcon,
	Plus,
	RefreshCcw,
	ShieldAlert,
} from "lucide-react";
import { useState } from "react";

import {
	KnowledgeFile,
	KnowledgeFileState,
	KnowledgeSource,
	getKnowledgeFileDisplayName,
} from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "~/components/ui/collapsible";
import { Label } from "~/components/ui/label";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export type FileNode = {
	name: string;
	path: string;
	file: KnowledgeFile | null;
	children?: FileNode[];
};

const getAllFiles = (node: FileNode): KnowledgeFile[] => {
	if (node.file) return [node.file];
	return [...node.children!.flatMap(getAllFiles)];
};

export default function FileTreeNode({
	node,
	level,
	source,
	onApproveFile,
	onReingestFile,
	setErrorDialogError,
	updateKnowledgeSource,
}: {
	node: FileNode;
	level: number;
	source: KnowledgeSource;
	onApproveFile: (approved: boolean, fileNode: FileNode) => void;
	onReingestFile: (file: KnowledgeFile) => void;
	setErrorDialogError: (error: string) => void;
	updateKnowledgeSource: (source: KnowledgeSource) => void;
}) {
	const [isOpen, setIsOpen] = useState(false);
	const hasChildren = node.children && node.children.length > 0;

	const allFiles = getAllFiles(node);
	const totalFiles = allFiles.length;
	const ingestingFiles = allFiles.filter(
		(file) => file.state === KnowledgeFileState.Ingesting
	).length;
	const ingestedFiles = allFiles.filter(
		(file) => file.state === KnowledgeFileState.Ingested
	).length;
	const excludedFiles = allFiles.filter(
		(file) => file.state === KnowledgeFileState.Unapproved
	).length;
	const selectedFiles = allFiles.filter((file) => file.approved).length;
	const errorFiles = allFiles.filter(
		(file) => file.state === KnowledgeFileState.Error
	).length;
	const unsupportedFiles = allFiles.filter(
		(file) => file.state === KnowledgeFileState.Unsupported
	).length;
	const totalSize = allFiles.reduce(
		(acc, file) => acc + (file.sizeInBytes || 0),
		0
	);

	const isFile = node.file !== null;
	const file = node.file!;

	const included =
		(source.filePathPrefixInclude?.some((prefix) =>
			node.path.startsWith(prefix)
		) &&
			!source.filePathPrefixExclude?.some((prefix) =>
				node.path.startsWith(prefix)
			)) ||
		(node.file && node.file.approved);

	const excluded = source.filePathPrefixExclude?.some((prefix) =>
		node.path.startsWith(prefix)
	);

	// We shouldn't allow user to toggle include button if its parent folder has been excluded. This is against the design from backend which is built from whitelist + blacklist where whitelist is preferred.
	// So if a folder is excluded, all its children should be excluded by default and the only way to include it is to remove the parent folder from the blacklist.
	const disableToggleButton =
		excluded && !source.filePathPrefixExclude?.includes(node.path);

	const toggleIncludeExcludeList = () => {
		// After files are approved/unapproved, we need to update the include/exclude list so that new files will be included/excluded from future syncs.
		let filePathPrefixInclude = source.filePathPrefixInclude;
		let filePathPrefixExclude = source.filePathPrefixExclude;
		if (included) {
			filePathPrefixInclude = source.filePathPrefixInclude?.filter(
				(path) => !path.startsWith(node.path)
			);
			filePathPrefixExclude = source.filePathPrefixExclude?.includes(node.path)
				? source.filePathPrefixExclude
				: [...(source.filePathPrefixExclude ?? []), node.path];
		} else {
			filePathPrefixInclude = source.filePathPrefixInclude?.includes(node.path)
				? source.filePathPrefixInclude
				: [...(source.filePathPrefixInclude ?? []), node.path];
			filePathPrefixExclude = source.filePathPrefixExclude?.filter(
				(path) => !path.startsWith(node.path)
			);
		}

		updateKnowledgeSource({
			...source,
			filePathPrefixInclude,
			filePathPrefixExclude,
		});
	};

	return (
		<div className={cn("border-l", level > 0 && "ml-4")}>
			<Collapsible open={isOpen} onOpenChange={setIsOpen}>
				<CollapsibleTrigger asChild>
					<div
						className={cn(
							"flex flex-row justify-between p-2 hover:bg-accent hover:text-accent-foreground",
							!isFile && "hover:cursor-pointer",
							"group"
						)}
					>
						<div className="flex flex-1 items-center justify-between truncate">
							<div className="flex items-center justify-center overflow-hidden">
								{hasChildren ? (
									isOpen ? (
										<>
											<ChevronDown className="mr-2 h-4 w-4 flex-shrink-0" />
											<FolderOpen className="mr-2 h-4 w-4 flex-shrink-0" />
										</>
									) : (
										<>
											<ChevronRight className="mr-2 h-4 w-4 flex-shrink-0" />
											<Folder className="mr-2 h-4 w-4 flex-shrink-0" />
										</>
									)
								) : (
									<File className="mr-2 h-4 w-4 flex-shrink-0" />
								)}
								{isFile ? (
									<Tooltip>
										<TooltipTrigger asChild>
											<a
												href={file.url}
												target="_blank"
												rel="noopener noreferrer"
												className={cn(
													"flex flex-1 overflow-hidden truncate hover:underline",
													(file.state === KnowledgeFileState.Unapproved ||
														file.state ===
															KnowledgeFileState.PendingApproval) &&
														"text-muted-foreground"
												)}
											>
												<span className="truncate">
													{
														getKnowledgeFileDisplayName(file, source)
															.displayName
													}
												</span>
											</a>
										</TooltipTrigger>
										<TooltipContent>
											{getKnowledgeFileDisplayName(file, source).displayName}
										</TooltipContent>
									</Tooltip>
								) : (
									<Tooltip>
										<TooltipTrigger asChild>
											<span
												className={cn(
													"flex truncate font-medium",
													selectedFiles === 0 && "text-muted-foreground"
												)}
											>
												{node.name}
											</span>
										</TooltipTrigger>
										<TooltipContent>{node.name}</TooltipContent>
									</Tooltip>
								)}
								{isFile ? (
									<div className="ml-2 flex flex-row items-center justify-center">
										{file.state === KnowledgeFileState.Ingesting ? (
											<LoadingSpinner className="h-4 w-4" />
										) : file.state === KnowledgeFileState.Ingested ? (
											<CheckIcon className="h-4 w-4 text-success" />
										) : file.state === KnowledgeFileState.Pending ? (
											<FileClock className="h-4 w-4" />
										) : file.state === KnowledgeFileState.Error ? (
											<CircleX className="h-4 w-4 text-destructive" />
										) : file.state === KnowledgeFileState.PendingApproval ||
										  file.state ===
												KnowledgeFileState.Unapproved ? null : file.state ===
										  KnowledgeFileState.Unsupported ? (
											<ShieldAlert className="text-danger h-4 w-4" />
										) : null}
									</div>
								) : (
									<div className="ml-4 flex flex-row items-center justify-center">
										{included ? (
											<span className="text-xs text-success">Included</span>
										) : excluded ? (
											<span className="text-xs text-muted-foreground">
												Excluded
											</span>
										) : null}
									</div>
								)}
							</div>
							{!disableToggleButton && (
								<div className="group ml-2 mr-2 flex items-center">
									<Button
										variant="ghost"
										size="icon"
										onClick={(e) => {
											e.stopPropagation();
											if (!isFile) {
												toggleIncludeExcludeList();
												return;
											}
											const approved = !file.approved;
											onApproveFile(approved, node);
											// we also need to add the file to exclude list if it is not approved so that it will be automatically excluded from future
											if (!approved) {
												updateKnowledgeSource({
													...source,
													filePathPrefixExclude: [
														...(source.filePathPrefixExclude ?? []),
														file.fileName,
													],
												});
											}
										}}
										className="group invisible items-center justify-center hover:bg-gray-200 group-hover:visible"
									>
										<Tooltip>
											<TooltipTrigger asChild>
												<div className="flex items-center justify-center">
													{included ? (
														<MinusIcon className="h-4 w-4 text-destructive" />
													) : (
														<Plus className="h-4 w-4" />
													)}
												</div>
											</TooltipTrigger>
											<TooltipContent>
												{included
													? "Exclude from Knowledge"
													: "Add to Knowledge"}
											</TooltipContent>
										</Tooltip>
									</Button>
								</div>
							)}
						</div>
						{node.file ? (
							<div className="flex items-center justify-center space-x-2">
								<div className="flex items-center justify-center">
									{node.file.state ===
									KnowledgeFileState.PendingApproval ? null : node.file
											.state === KnowledgeFileState.Unapproved ? (
										<span className="text-xs text-muted-foreground">
											Excluded
										</span>
									) : node.file.state === KnowledgeFileState.Ingesting ? (
										<div className="flex items-center justify-center text-warning">
											<Label className="text-xs">Ingesting</Label>
										</div>
									) : node.file.state === KnowledgeFileState.Pending ? (
										<div className="flex items-center justify-center">
											<Label className="text-xs">Pending</Label>
										</div>
									) : node.file.state === KnowledgeFileState.Error ? (
										<div className="group flex items-center justify-center text-destructive">
											<Label className="text-xs text-destructive group-hover:hidden">
												Error
											</Label>

											<Tooltip>
												<TooltipTrigger asChild>
													<Button
														variant="ghost"
														size="icon"
														className="hidden items-center justify-center text-destructive group-hover:block"
														onClick={async () => {
															if (!node.file) return;
															await onReingestFile(node.file);
														}}
													>
														<RefreshCcw className="m-auto h-4 w-4 text-destructive" />
													</Button>
												</TooltipTrigger>
												<TooltipContent>Reingest</TooltipContent>
											</Tooltip>
											<Tooltip>
												<TooltipTrigger asChild>
													<Button
														variant="ghost"
														size="icon"
														className="hidden items-center justify-center text-destructive group-hover:block"
														onClick={() => {
															setErrorDialogError(node.file?.error ?? "");
														}}
													>
														<Eye className="m-auto h-4 w-4 text-destructive" />
													</Button>
												</TooltipTrigger>
												<TooltipContent>View Error</TooltipContent>
											</Tooltip>
										</div>
									) : node.file.state === KnowledgeFileState.Ingested ? (
										<div className="flex items-center justify-center text-success">
											<Label className="text-xs">Ingested</Label>
										</div>
									) : node.file.state === KnowledgeFileState.Unsupported ? (
										<div className="flex items-center justify-center">
											<Tooltip>
												<TooltipTrigger asChild>
													<Label className="cursor-pointer text-xs">
														Unsupported
													</Label>
												</TooltipTrigger>
												<TooltipContent>{node.file.error}</TooltipContent>
											</Tooltip>
										</div>
									) : null}
								</div>
								<span className="flex items-center justify-center text-xs text-muted-foreground">
									{node.file.sizeInBytes
										? node.file.sizeInBytes > 1024 * 1024
											? (node.file.sizeInBytes / (1024 * 1024)).toFixed(2) +
												" MB"
											: node.file.sizeInBytes > 1024
												? (node.file.sizeInBytes / 1024).toFixed(2) + " KB"
												: node.file.sizeInBytes + " Bytes"
										: "0 Bytes"}
								</span>
							</div>
						) : (
							<div className="flex items-center justify-center space-x-2 text-muted-foreground">
								<div className="mr-2 items-center justify-center whitespace-nowrap text-xs">
									<span className="text-xs font-medium">
										{ingestingFiles > 0 && (
											<>
												<span className="text-warning">{`${ingestingFiles}`}</span>
												<span>{` Ingesting, `}</span>
											</>
										)}
										{ingestedFiles > 0 && (
											<>
												<span className="text-success">{`${ingestedFiles}`}</span>
												<span>{` Ingested, `}</span>
											</>
										)}
										{errorFiles > 0 && (
											<>
												<span className="text-destructive">{`${errorFiles}`}</span>
												<span>{` Err, `}</span>
											</>
										)}
										{unsupportedFiles > 0 && (
											<>
												<span className="text-warning">{`${unsupportedFiles}`}</span>
												<span>{` Unsupported, `}</span>
											</>
										)}
										{selectedFiles > 0 && (
											<span>{`${selectedFiles} Inc, `}</span>
										)}
										{excludedFiles > 0 && (
											<span>{`${excludedFiles} Exc, `}</span>
										)}
										<span>{`${totalFiles} Total`}</span>
									</span>
								</div>
								<div className="whitespace-nowrap text-xs">
									{totalSize > 1024 * 1024
										? (totalSize / (1024 * 1024)).toFixed(2) + " MB"
										: totalSize > 1024
											? (totalSize / 1024).toFixed(2) + " KB"
											: totalSize + " Bytes"}
								</div>
							</div>
						)}
					</div>
				</CollapsibleTrigger>
				{hasChildren && (
					<CollapsibleContent>
						{node
							.children!.sort((a, b) => {
								if (a.file === null && b.file !== null) return -1;
								if (a.file !== null && b.file === null) return 1;
								return a.path.localeCompare(b.path);
							})
							.map((child, index) => (
								<FileTreeNode
									key={index}
									node={child}
									level={level + 1}
									source={source}
									onApproveFile={onApproveFile}
									onReingestFile={onReingestFile}
									setErrorDialogError={setErrorDialogError}
									updateKnowledgeSource={updateKnowledgeSource}
								/>
							))}
					</CollapsibleContent>
				)}
			</Collapsible>
		</div>
	);
}
