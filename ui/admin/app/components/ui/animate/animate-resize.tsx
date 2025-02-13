import { ComponentProps } from "react";
import useMeasure from "react-use-measure";

import { cn } from "~/lib/utils";

import { Animate } from "~/components/ui/animate";

type Axis = "width" | "height";

export function AnimateResize({
	children,
	axis,
	classes = {},
	...restProps
}: {
	children: React.ReactNode;
	axis: Axis | Axis[];
	classes?: { container?: string; content?: string };
} & ComponentProps<typeof Animate.div>) {
	const [ref, bounds] = useMeasure();

	const _axis = Array.isArray(axis) ? axis : [axis];

	const width = _axis.includes("width") ? bounds.width : undefined;
	const height = _axis.includes("height") ? bounds.height : undefined;

	return (
		<Animate.div
			animate={{ width, height }}
			className={cn("flex overflow-hidden", classes.container)}
			{...restProps}
		>
			<div ref={ref} className={classes.content}>
				{children}
			</div>
		</Animate.div>
	);
}
