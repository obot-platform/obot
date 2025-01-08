import { ClipboardCheckIcon, ClipboardIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type CopyTextProps = {
    text: string;
    displayText?: string;
    className?: string;
    holdStatusDelay?: number;
    hideText?: boolean;
    hideIcon?: boolean;
    classNames?: {
        root?: string;
        textWrapper?: string;
        text?: string;
        icon?: string;
    };
};

export function CopyText({
    text,
    displayText = text,
    className,
    holdStatusDelay,
    hideText,
    hideIcon,
    classNames = {},
}: CopyTextProps) {
    const [isCopied, setIsCopied] = useState(false);

    useEffect(() => {
        if (!isCopied || !holdStatusDelay) return;

        const timeout = setTimeout(() => setIsCopied(false), holdStatusDelay);

        return () => clearTimeout(timeout);
    }, [isCopied, holdStatusDelay]);

    return (
        <div
            className={cn(
                "flex items-center gap-2 bg-accent rounded-md w-fit overflow-hidden",
                className,
                classNames.root
            )}
        >
            {!hideText && (
                <Tooltip>
                    <TooltipTrigger
                        type="button"
                        onClick={() => handleCopy(text)}
                        className={cn(
                            "decoration-dotted underline-offset-4 underline text-ellipsis overflow-hidden text-nowrap",
                            classNames.textWrapper
                        )}
                    >
                        <p
                            className={cn(
                                "truncate break-words p-2",
                                classNames.text
                            )}
                        >
                            {displayText}
                        </p>
                    </TooltipTrigger>

                    <TooltipContent>
                        <b>Copy: </b>
                        {text}
                    </TooltipContent>
                </Tooltip>
            )}

            {!hideIcon && (
                <Button
                    size="icon"
                    onClick={() => handleCopy(text)}
                    className={cn("aspect-square", classNames.icon)}
                    variant="ghost"
                    type="button"
                >
                    {isCopied ? (
                        <ClipboardCheckIcon className="text-success" />
                    ) : (
                        <ClipboardIcon />
                    )}
                </Button>
            )}
        </div>
    );

    async function handleCopy(text: string) {
        try {
            await navigator.clipboard.writeText(text);
            toast.success("Copied to clipboard");
            setIsCopied(true);
        } catch (error) {
            console.error("Failed to copy text: ", error);
            toast.error("Failed to copy text");
        }
    }
}
