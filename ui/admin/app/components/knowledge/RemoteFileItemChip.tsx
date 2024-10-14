import { XIcon } from "lucide-react";

import { RemoteKnowledgeSourceType } from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import RemoteFileAvatar from "./RemoteFileAvatar";

type RemoteFileItemProps = {
    displayName: string;
    url: string;
    onAction?: () => void;
    actionIcon?: React.ReactNode;
    isLoading?: boolean;
    error?: string;
    statusIcon?: React.ReactNode;
    remoteKnowledgeSourceType: RemoteKnowledgeSourceType;
} & React.HTMLAttributes<HTMLDivElement>;

export default function RemoteFileItemChip({
    displayName,
    url,
    className,
    onAction,
    actionIcon,
    isLoading,
    error,
    statusIcon,
    remoteKnowledgeSourceType,
    ...props
}: RemoteFileItemProps) {
    return (
        <TooltipProvider>
            <Tooltip>
                {error && <TooltipContent>{error}</TooltipContent>}

                <TooltipTrigger asChild>
                    <div
                        className={cn(
                            "flex justify-between flex-nowrap items-center gap-4 rounded-lg px-2 border w-full",
                            {
                                "bg-destructive-background border-destructive text-foreground cursor-pointer":
                                    error,
                                "grayscale opacity-60": isLoading,
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
                        <a
                            href={url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex flex-col overflow-hidden flex-auto hover:underline"
                        >
                            <TypographyP className="w-full overflow-hidden text-ellipsis">
                                {displayName}
                            </TypographyP>
                        </a>

                        {statusIcon}

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
                                        <XIcon className="w-4 h-4" />
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
