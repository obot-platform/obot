import { Loader2 } from "lucide-react";
import { ComponentProps } from "react";

import { cn } from "~/lib/utils";

export interface LoadingSpinnerProps extends ComponentProps<typeof Loader2> {
	size?: number;
	className?: string;
	fillContainer?: boolean;
	classNames?: {
		root?: string;
		container?: string;
	};
}

export const LoadingSpinner = ({
	className,
	fillContainer,
	classNames = {},
	...props
}: LoadingSpinnerProps) => {
	const content = (
		<Loader2 className={cn("animate-spin", className)} {...props} />
	);

	return fillContainer ? (
		<div
			className={cn(
				"flex h-full min-w-fit flex-auto items-center justify-center",
				classNames.container
			)}
		>
			{content}
		</div>
	) : (
		content
	);
};
