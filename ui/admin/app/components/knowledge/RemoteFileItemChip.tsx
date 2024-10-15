import { PlusIcon } from "lucide-react";
import { useEffect, useState } from "react";

import {
    KnowledgeFile,
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
    remoteKnowledgeSourceType: RemoteKnowledgeSourceType;
    approveFile: (file: KnowledgeFile, approved: boolean) => void;
    subTitle?: string;
} & React.HTMLAttributes<HTMLDivElement>;

export default function RemoteFileItemChip({
    file,
    className,
    error,
    remoteKnowledgeSourceType,
    subTitle,
    approveFile,
    ...props
}: RemoteFileItemProps) {
    const [isApproved, setIsApproved] = useState(false);
    useEffect(() => {
        setIsApproved(file.approved!);
    }, [file.approved]);
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
                                "grayscale opacity-60": !isApproved,
                            },
                            className
                        )}
                        {...props}
                    >
                        <RemoteFileAvatar
                            remoteKnowledgeSourceType={
                                remoteKnowledgeSourceType
                            }
                        />
                        <div className="flex flex-col overflow-hidden flex-auto">
                            <a
                                href={file.fileDetails.url}
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

                        {isApproved ? (
                            // eslint-disable-next-line
                            <div
                                className="hover:cursor-pointer"
                                onClick={() => {
                                    setIsApproved(false);
                                    approveFile(file, false);
                                }}
                            >
                                <FileStatusIcon status={file.ingestionStatus} />
                            </div>
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
                    </div>
                </TooltipTrigger>
            </Tooltip>
        </TooltipProvider>
    );
}
