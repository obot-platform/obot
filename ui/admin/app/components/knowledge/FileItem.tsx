import { FileIcon, PlusIcon, TrashIcon } from "lucide-react";
import { useState } from "react";

import { KnowledgeFile } from "~/lib/model/knowledge";
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
    error,
    approveFile,
    ...props
}: FileItemProps) {
    const [isApproved, setIsApproved] = useState(file.approved);
    return (
        <TooltipProvider>
            <Tooltip>
                {error && <TooltipContent>{error}</TooltipContent>}

                <TooltipTrigger asChild>
                    <div
                        className={cn(
                            "flex justify-between flex-nowrap items-center gap-4 rounded-lg px-2 border w-full",
                            {
                                "bg-destructive-background border-destructive text-foreground":
                                    error,
                                "grayscale opacity-60":
                                    isLoading || !isApproved,
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

                        {isApproved ? (
                            <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                    setIsApproved(false);
                                    approveFile(file, false);
                                }}
                            >
                                <FileStatusIcon status={file.ingestionStatus} />
                            </Button>
                        ) : (
                            <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                    setIsApproved(true);
                                    approveFile(file, true);
                                }}
                            >
                                <PlusIcon className="w-4 h-4" />
                            </Button>
                        )}

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
