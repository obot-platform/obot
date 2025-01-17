import { Animate } from "~/components/ui/animate";

type RotateProps = {
	children: React.ReactNode;
	active: boolean;
	degrees?: number;
};

export function Rotate({ children, active, degrees = 90 }: RotateProps) {
	return (
		<Animate.div
			initial={{ rotate: 0 }}
			animate={{ rotate: active ? degrees : 0 }}
		>
			{children}
		</Animate.div>
	);
}
