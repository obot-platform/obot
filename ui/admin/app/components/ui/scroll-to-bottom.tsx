import { ArrowDown } from "lucide-react";
import React from "react";

import { useDebouncedValue } from "~/hooks/useDebounce";

import { Button } from "./button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "./tooltip";

type ScrollToBottomProps = {
    scrollContainerEl: HTMLDivElement | null;
    disabled?: boolean;
    onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
    offset?: number;
    delay?: number;
    behavior?: ScrollBehavior;
};

function ScrollToBottom({
    scrollContainerEl,
    disabled = false,
    onClick,
    delay = 500,
    behavior = "instant",
}: ScrollToBottomProps) {
    const isScrolledToBottom = getIsScrolledToBottom();
    const debounced = useDebouncedValue(isScrolledToBottom, delay);

    const calc = () => {
        if (isScrolledToBottom) return false;
        return !debounced;
    };

    return (
        !disabled &&
        calc() && (
            <TooltipProvider>
                <Tooltip delayDuration={300}>
                    <TooltipTrigger asChild>
                        <Button
                            size="icon"
                            variant="ghost"
                            className="absolute bottom-2 left-1/2 -translate-x-1/2 bg-background rounded-full border"
                            onClick={(e) => {
                                scrollToBottom();
                                onClick?.(e);
                            }}
                        >
                            <ArrowDown className="w-6 h-6" />
                        </Button>
                    </TooltipTrigger>

                    <TooltipContent>Scroll to bottom</TooltipContent>
                </Tooltip>
            </TooltipProvider>
        )
    );

    function getIsScrolledToBottom() {
        if (!scrollContainerEl) return false;

        const { scrollTop, scrollHeight, clientHeight } = scrollContainerEl;
        return scrollHeight - scrollTop - clientHeight === 0;
    }

    function scrollToBottom() {
        if (!scrollContainerEl) return;

        scrollContainerEl.scrollTo({
            top: scrollContainerEl.scrollHeight,
            behavior,
        });
    }
}

export { ScrollToBottom };
