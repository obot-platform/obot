import useMeasure from "react-use-measure";

import { cn } from "~/lib/utils";

import { Animate } from "~/components/ui/animate";

type Axis = "width" | "height";

const defaultAxis: Axis[] = ["width", "height"];

export function AnimateResize({
	children,
	axis = defaultAxis,
	classes = {},
}: {
	children: React.ReactNode;
	axis?: Axis | Axis[];
	classes?: { container?: string; content?: string };
}) {
	const [ref, bounds] = useMeasure();

	const _axis = Array.isArray(axis) ? axis : [axis];

	const width = _axis.includes("width") ? bounds.width : undefined;
	const height = _axis.includes("height") ? bounds.height : undefined;

	return (
		<Animate.div
			animate={{ width, height }}
			className={cn("flex overflow-hidden", classes.container)}
		>
			<div ref={ref} className={classes.content}>
				{children}
			</div>
		</Animate.div>
	);
}
