import { FileIcon, PlusIcon, TrashIcon } from "lucide-react";

import { KnowledgeFile, KnowledgeFileState } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import { LoadingSpinner } from "../ui/LoadingSpinner";
import FileStatusIcon from "./FileStatusIcon";

type FileItemProps = {
    file: KnowledgeFile;
    onAction?: () => void;
    actionIcon?: React.ReactNode;
    isLoading?: boolean;
    error?: string;
    approveFile: (file: KnowledgeFile, approved: boolean) => void;
} & React.HTMLAttributes<HTMLDivElement>;

function FileItem({
    className,
    file,
    onAction,
    actionIcon,
    isLoading,
    approveFile,
    ...props
}: FileItemProps) {
    return (
        <TooltipProvider>
            <Tooltip>
                <TooltipTrigger asChild>
                    <div
                        className={cn(
                            "flex justify-between flex-nowrap items-center gap-4 rounded-lg px-2 border w-full",
                            {
                                "grayscale opacity-60": isLoading,
                            },
                            className
                        )}
                        {...props}
                    >
                        <FileIcon className="w-4 h-4" />

                        <div className="flex flex-col overflow-auto flex-auto">
                            <TypographyP className="flex overflow-x-auto text-ellipsis whitespace-nowrap">
                                {file?.fileName}
                            </TypographyP>
                        </div>

                        <div className="mr-2">
                            <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                    if (
                                        file.state === KnowledgeFileState.Error
                                    ) {
                                        KnowledgeService.reingestFile(
                                            file.agentID,
                                            file.knowledgeSourceID,
                                            file.id
                                        );
                                        return;
                                    }
                                }}
                            >
                                <FileStatusIcon file={file} />
                            </Button>
                        </div>

                        {isLoading ? (
                            <Button disabled variant="ghost" size="icon">
                                <LoadingSpinner className="w-4 h-4" />
                            </Button>
                        ) : (
                            onAction && (
                                <Button
                                    variant="ghost"
                                    size="icon"
                                    onClick={onAction}
                                >
                                    {actionIcon ? (
                                        actionIcon
                                    ) : (
                                        <TrashIcon className="w-4 h-4" />
                                    )}
                                </Button>
                            )
                        )}
                    </div>
                </TooltipTrigger>
            </Tooltip>
        </TooltipProvider>
    );
}

export { FileItem as FileChip };
