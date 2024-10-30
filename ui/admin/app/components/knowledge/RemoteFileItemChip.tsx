import {
    KnowledgeFile,
    KnowledgeFileState,
    RemoteKnowledgeSourceType,
    getRemoteFileDisplayName,
} from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import { Button } from "../ui/button";
import FileStatusIcon from "./FileStatusIcon";
import RemoteFileAvatar from "./RemoteFileAvatar";

type RemoteFileItemProps = {
    file: KnowledgeFile;
    error?: string;
    knowledgeSourceType: RemoteKnowledgeSourceType;
    approveFile: (file: KnowledgeFile, approved: boolean) => void;
    reingestFile: (file: KnowledgeFile) => void;
    subTitle?: string;
} & React.HTMLAttributes<HTMLDivElement>;

export default function RemoteFileItemChip({
    file,
    className,
    error,
    knowledgeSourceType,
    subTitle,
    approveFile,
    reingestFile,
    ...props
}: RemoteFileItemProps) {
    return (
        <TooltipProvider>
            <Tooltip>
                {error && <TooltipContent>{error}</TooltipContent>}

                <TooltipTrigger asChild>
                    <div
                        className={cn(
                            "flex justify-between flex-nowrap items-center gap-4 rounded-lg px-2 border w-full hover:cursor-pointer",
                            {
                                "bg-destructive-background border-destructive hover:cursor-pointer":
                                    error,
                                "grayscale opacity-60":
                                    file.state ===
                                        KnowledgeFileState.PendingApproval ||
                                    file.state ===
                                        KnowledgeFileState.Unapproved,
                            },
                            className
                        )}
                        {...props}
                    >
                        <RemoteFileAvatar
                            knowledgeSourceType={knowledgeSourceType}
                        />
                        <div className="flex flex-col overflow-hidden flex-auto">
                            <a
                                href={file.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="flex flex-col overflow-hidden flex-auto hover:underline"
                                onClick={(e) => {
                                    e.stopPropagation();
                                }}
                            >
                                <TypographyP className="w-full overflow-hidden text-ellipsis">
                                    {getRemoteFileDisplayName(file)}
                                </TypographyP>
                            </a>
                            <span className="text-gray-400 text-xs">
                                {subTitle}
                            </span>
                        </div>

                        <div className="mr-2">
                            <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                    if (
                                        file.state ===
                                        KnowledgeFileState.PendingApproval
                                    ) {
                                        approveFile(file, true);
                                    }

                                    if (
                                        file.state ===
                                        KnowledgeFileState.Ingested
                                    ) {
                                        approveFile(file, false);
                                    }

                                    if (
                                        file.state === KnowledgeFileState.Error
                                    ) {
                                        reingestFile(file);
                                        return;
                                    }
                                }}
                            >
                                <FileStatusIcon file={file} />
                            </Button>
                        </div>
                    </div>
                </TooltipTrigger>
            </Tooltip>
        </TooltipProvider>
    );
}
