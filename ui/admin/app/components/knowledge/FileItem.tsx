import { FileIcon, XIcon } from "lucide-react";

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

type FileItemProps = {
    fileName: string;
    onAction?: () => void;
    actionIcon?: React.ReactNode;
    isLoading?: boolean;
    error?: string;
    statusIcon?: React.ReactNode;
} & React.HTMLAttributes<HTMLDivElement>;

function FileItem({
    fileName,
    className,
    onAction,
    actionIcon,
    isLoading,
    error,
    statusIcon,
    ...props
}: FileItemProps) {
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
                        <FileIcon className="w-4 h-4" />

                        <div className="flex flex-col overflow-auto flex-auto">
                            <TypographyP className="flex overflow-x-auto text-ellipsis whitespace-nowrap">
                                {fileName}
                            </TypographyP>
                        </div>

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

export { FileItem as FileChip };
