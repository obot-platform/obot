import { CheckIcon, InfoIcon, XIcon } from "lucide-react";

import {
    IngestionStatus,
    KnowledgeIngestionStatus,
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

const ingestionIcons = {
    [IngestionStatus.Queued]: [LoadingSpinner, ""],
    [IngestionStatus.Finished]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Completed]: [LoadingSpinner, ""],
    [IngestionStatus.Skipped]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Starting]: [LoadingSpinner, ""],
    [IngestionStatus.Failed]: [XIcon, "text-destructive"],
    [IngestionStatus.Unsupported]: [InfoIcon, "text-yellow-500"],
} as const;

type FileStatusIconProps = {
    status?: KnowledgeIngestionStatus;
} & React.HTMLAttributes<HTMLDivElement>;

const FileStatusIcon: React.FC<FileStatusIconProps> = ({ status }) => {
    if (!status || !status.status) return null;
    const [Icon, className] = ingestionIcons[status.status];

    return (
        <div className={cn("flex items-center mr-2", className)}>
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
                        {getMessage(status.status, status.msg, status.error)}
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );
};

export default FileStatusIcon;
