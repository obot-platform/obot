import { CheckIcon, PlusIcon, RotateCcwIcon } from "lucide-react";

import {
    KnowledgeFile,
    KnowledgeFileState,
    getMessage,
} from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import { LoadingSpinner } from "../ui/LoadingSpinner";

const fileStateIcons: Record<KnowledgeFileState, [React.ElementType, string]> =
    {
        [KnowledgeFileState.PendingApproval]: [PlusIcon, ""],
        [KnowledgeFileState.Pending]: [LoadingSpinner, ""],
        [KnowledgeFileState.Ingesting]: [LoadingSpinner, ""],
        [KnowledgeFileState.Ingested]: [CheckIcon, "text-green-500"],
        [KnowledgeFileState.Error]: [RotateCcwIcon, "text-destructive"],
        [KnowledgeFileState.Unapproved]: [PlusIcon, "text-warning"],
    } as const;

type FileStatusIconProps = {
    file: KnowledgeFile;
};

const FileStatusIcon: React.FC<FileStatusIconProps> = ({ file }) => {
    const [Icon, className] = fileStateIcons[file.state];

    return (
        <div className={cn("flex items-center", className)}>
            <TooltipProvider>
                <Tooltip>
                    <TooltipTrigger asChild>
                        <div>
                            {Icon === LoadingSpinner ? (
                                <LoadingSpinner
                                    className={cn("w-4 h-4", className)}
                                />
                            ) : (
                                <Icon className={cn("w-4 h-4", className)} />
                            )}
                        </div>
                    </TooltipTrigger>
                    <TooltipContent className="whitespace-normal break-words max-w-[300px] max-h-full">
                        {getMessage(file.state, file.error)}
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );
};

export default FileStatusIcon;
