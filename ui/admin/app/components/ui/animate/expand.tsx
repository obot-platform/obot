import { Animate } from "~/components/ui/animate";

export function ExpandAndCollapse({
	children,
	active,
}: {
	children: React.ReactNode;
	active: boolean;
}) {
	return (
		<Animate.div
			initial={false}
			animate={
				active
					? { height: "auto", opacity: 1, visibility: "visible" }
					: { height: 0, opacity: 0, visibility: "hidden" }
			}
		>
			{children}
		</Animate.div>
	);
}
