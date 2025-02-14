import * as ScrollAreaPrimitive from "@radix-ui/react-scroll-area";
import * as React from "react";

import { cn } from "~/lib/utils";
import { isScrolledToBottom } from "~/lib/utils/isScrolledToBottom";

import { ScrollToBottom } from "~/components/ui/scroll-to-bottom";
import { useOnResize } from "~/hooks/useOnResize";

// note: I opted not to guess how to implement other scroll directions (i.e. 'top', 'left', 'right')
// because I don't think it's necessary for the current use case. If you do need it, feel free to
// implement it and submit a PR.

const ScrollArea = React.forwardRef<
	React.ElementRef<typeof ScrollAreaPrimitive.Root>,
	React.ComponentPropsWithoutRef<typeof ScrollAreaPrimitive.Root> & {
		startScrollAt?: "bottom";
		enableScrollStick?: "bottom";
		enableScrollTo?: "bottom";
		orientation?: "vertical" | "horizontal";
		classNames?: {
			root?: string;
			viewport?: string;
			content?: string;
		};
	}
>((props, ref) => {
	const {
		className,
		children,
		startScrollAt,
		enableScrollTo,
		enableScrollStick,
		orientation = "vertical",
		classNames = {},
		...rootProps
	} = props;

	const [viewportEl, setViewportEl] = React.useState<HTMLDivElement | null>(
		null
	);
	const viewportRef = React.useRef<HTMLDivElement | null>(null);
	const [shouldStickToBottom, setShouldStickToBottom] = React.useState(
		enableScrollStick === "bottom" && startScrollAt === "bottom"
	);

	React.useEffect(() => {
		if (startScrollAt === "bottom") {
			viewportRef.current?.scrollTo({
				top: viewportRef.current.scrollHeight,
				behavior: "instant",
			});
		}
	}, [startScrollAt]);

	const contentRef = React.useRef<HTMLDivElement | null>(null);

	useOnResize(
		contentRef,
		React.useCallback(() => {
			if (shouldStickToBottom && enableScrollStick === "bottom") {
				const el = viewportRef.current;
				if (!el) return;

				const maxScrollHeight = el.scrollHeight - el.clientHeight;
				el.scrollTop = maxScrollHeight;
			}
		}, [enableScrollStick, shouldStickToBottom])
	);

	const initRef = React.useCallback((node: HTMLDivElement | null) => {
		setViewportEl(node);
		viewportRef.current = node;
	}, []);

	return (
		<ScrollAreaPrimitive.Root
			ref={ref}
			className={cn("relative overflow-hidden", className, classNames.root)}
			{...rootProps}
		>
			<ScrollAreaPrimitive.Viewport
				className={cn(
					// "[&>div]:!block" is a workaround to fix width expansion issues caused by the viewport
					// setting `display: table` in the `ScrollAreaPrimitive.Viewport` component.
					// This is a known issue with Radix UI ScrollArea.
					// https://github.com/radix-ui/primitives/issues/2722
					"h-full max-h-[inherit] w-full max-w-[inherit] scroll-smooth rounded-[inherit] [&>div]:!block",
					classNames.viewport
				)}
				ref={initRef}
				onWheel={(e) => {
					if (e.deltaY < 0) {
						setShouldStickToBottom(false);
					} else if (viewportRef.current) {
						setShouldStickToBottom(isScrolledToBottom(viewportRef.current));
					}
				}}
			>
				<div ref={contentRef} className={classNames.content}>
					{children}
				</div>
				{enableScrollTo === "bottom" && (
					<ScrollToBottom
						behavior="smooth"
						onClick={() => setShouldStickToBottom(true)}
						scrollContainerEl={viewportEl}
						disabled={shouldStickToBottom}
					/>
				)}
			</ScrollAreaPrimitive.Viewport>
			<ScrollBar orientation={orientation} />
			<ScrollAreaPrimitive.Corner />
		</ScrollAreaPrimitive.Root>
	);
});
ScrollArea.displayName = ScrollAreaPrimitive.Root.displayName;

const ScrollBar = React.forwardRef<
	React.ElementRef<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>,
	React.ComponentPropsWithoutRef<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>
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
