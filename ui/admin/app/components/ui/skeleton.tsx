import { cn } from "~/lib/utils";

function Skeleton({
	className,
	children,
	...props
}: React.HTMLAttributes<HTMLDivElement>) {
	return (
		<div
			className={cn("animate-pulse rounded-md bg-primary/10", className)}
			{...props}
		>
			{children && <div className="opacity-0">{children}</div>}
		</div>
	);
}

export { Skeleton };
