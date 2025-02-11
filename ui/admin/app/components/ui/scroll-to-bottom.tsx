import { ArrowDown } from "lucide-react";
import React, { useEffect, useState } from "react";

import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type ScrollToBottomProps = {
	scrollContainerEl: HTMLElement | null;
	disabled?: boolean;
	onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
	offset?: number;
	behavior?: ScrollBehavior;
};

function ScrollToBottom({
	scrollContainerEl,
	disabled = false,
	onClick,
	behavior = "instant",
}: ScrollToBottomProps) {
	const [isScrolledToBottom, setIsScrolledToBottom] = useState(false);

	useEffect(() => {
		if (!scrollContainerEl) return;

		const handler = () => {
			setIsScrolledToBottom(getIsScrolledToBottom(scrollContainerEl));
		};

		scrollContainerEl.addEventListener("scroll", handler);

		return () => scrollContainerEl.removeEventListener("scroll", handler);
	}, [scrollContainerEl]);

	return (
		!disabled &&
		scrollContainerEl &&
		!isScrolledToBottom && (
			<Tooltip delayDuration={300}>
				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="ghost"
						className="absolute bottom-2 left-1/2 -translate-x-1/2 rounded-full border bg-background"
						onClick={(e) => {
							scrollToBottom(scrollContainerEl, behavior);
							onClick?.(e);
						}}
					>
						<ArrowDown className="h-6 w-6" />
					</Button>
				</TooltipTrigger>

				<TooltipContent>Scroll to bottom</TooltipContent>
			</Tooltip>
		)
	);
}

function scrollToBottom(container: HTMLElement, behavior: ScrollBehavior) {
	if (!container) return;

	container.scrollTo({
		top: container.scrollHeight,
		behavior,
	});
}

function getIsScrolledToBottom(container: HTMLElement) {
	if (!container) return false;

	const { scrollTop, scrollHeight, clientHeight } = container;
	return scrollHeight - scrollTop - clientHeight < 1;
}

export { ScrollToBottom };
