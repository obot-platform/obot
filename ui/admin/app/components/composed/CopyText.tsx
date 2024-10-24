import { ClipboardCheckIcon, ClipboardIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function CopyText({
    text,
    displayText = text,
    className,
}: {
    text: string;
    displayText?: string;
    className?: string;
}) {
    const [isCopied, setIsCopied] = useState(false);

    useEffect(() => {
        if (!isCopied) return;

        const timeout = setTimeout(() => setIsCopied(false), 10000);

        return () => clearTimeout(timeout);
    }, [isCopied]);

    return (
        <div
            className={cn(
                "flex items-center gap-2 bg-secondary rounded-md w-fit",
                className
            )}
        >
            <TooltipProvider>
                <Tooltip>
                    <TooltipTrigger
                        type="button"
                        onClick={() => handleCopy(text)}
                        className="decoration-dotted underline-offset-4 underline text-ellipsis overflow-hidden text-nowrap"
                    >
                        <TypographyP className="truncate break-words p-2">
                            {displayText}
                        </TypographyP>
                    </TooltipTrigger>

                    <TooltipContent>
                        <b>Copy: </b>
                        {text}
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>

            <Button
                size="icon"
                onClick={() => handleCopy(text)}
                className="aspect-square"
                variant="ghost"
                type="button"
            >
                {isCopied ? (
                    <ClipboardCheckIcon className="text-success" />
                ) : (
                    <ClipboardIcon />
                )}
            </Button>
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
