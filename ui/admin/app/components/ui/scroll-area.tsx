import * as ScrollAreaPrimitive from "@radix-ui/react-scroll-area";
import * as React from "react";

import { cn } from "~/lib/utils";

import { ScrollToBottom } from "./scroll-to-bottom";

// note: I opted not to guess how to implement other scroll directions (i.e. 'top', 'left', 'right')
// because I don't think it's necessary for the current use case. If you do need it, feel free to
// implement it and submit a PR.

const ScrollArea = React.forwardRef<
    React.ElementRef<typeof ScrollAreaPrimitive.Root>,
    React.ComponentPropsWithoutRef<typeof ScrollAreaPrimitive.Root> & {
        startScrollAt?: "bottom";
        enableScrollStick?: "bottom";
        enableScrollTo?: "bottom";
    }
>((props, ref) => {
    const {
        className,
        children,
        startScrollAt,
        enableScrollTo,
        enableScrollStick,
        ...rootProps
    } = props;

    const viewportRef = React.useRef<HTMLDivElement | null>(null);
    const [shouldStickToBottom, setShouldStickToBottom] = React.useState(
        enableScrollStick === "bottom"
    );

    React.useEffect(() => {
        if (startScrollAt === "bottom") {
            viewportRef.current?.scrollTo({
                top: viewportRef.current.scrollHeight,
                behavior: "instant",
            });
        }
    }, [startScrollAt]);

    const deferredChildren = React.useDeferredValue(children);

    React.useEffect(() => {
        if (shouldStickToBottom && enableScrollStick === "bottom") {
            viewportRef.current?.scrollTo({
                top: viewportRef.current.scrollHeight,
                behavior: "instant",
            });
        }
    }, [shouldStickToBottom, enableScrollStick, deferredChildren]);

    return (
        <ScrollAreaPrimitive.Root
            ref={ref}
            className={cn("relative overflow-hidden", className)}
            {...rootProps}
        >
            <ScrollAreaPrimitive.Viewport
                className="h-full w-full rounded-[inherit] max-h-[inherit]"
                ref={viewportRef}
                onScroll={(e) =>
                    setShouldStickToBottom(isScrolledToBottom(e.currentTarget))
                }
            >
                {children}
                {enableScrollTo === "bottom" && (
                    <ScrollToBottom
                        onClick={() => setShouldStickToBottom(true)}
                        scrollContainerEl={viewportRef.current}
                        disabled={shouldStickToBottom}
                    />
                )}
            </ScrollAreaPrimitive.Viewport>
            <ScrollBar />
            <ScrollAreaPrimitive.Corner />
        </ScrollAreaPrimitive.Root>
    );
});
ScrollArea.displayName = ScrollAreaPrimitive.Root.displayName;

function isScrolledToBottom(container: HTMLDivElement) {
    const { scrollTop, scrollHeight, clientHeight } = container;
    return scrollHeight - scrollTop <= clientHeight;
}

const ScrollBar = React.forwardRef<
    React.ElementRef<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>,
    React.ComponentPropsWithoutRef<
        typeof ScrollAreaPrimitive.ScrollAreaScrollbar
    >
>(({ className, orientation = "vertical", ...props }, ref) => (
    <ScrollAreaPrimitive.ScrollAreaScrollbar
        ref={ref}
        orientation={orientation}
        className={cn(
            "flex touch-none select-none transition-colors",
            orientation === "vertical" &&
                "h-full w-2.5 border-l border-l-transparent p-[1px]",
            orientation === "horizontal" &&
                "h-2.5 flex-col border-t border-t-transparent p-[1px]",
            className
        )}
        {...props}
    >
        <ScrollAreaPrimitive.ScrollAreaThumb className="relative flex-1 rounded-full bg-border" />
    </ScrollAreaPrimitive.ScrollAreaScrollbar>
));
ScrollBar.displayName = ScrollAreaPrimitive.ScrollAreaScrollbar.displayName;

export { ScrollArea, ScrollBar };
