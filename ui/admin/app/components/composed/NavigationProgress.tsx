import { useEffect, useState } from "react";
import { useNavigation } from "react-router";

import { cn } from "~/lib/utils";

import { Animate, AnimatePresence } from "~/components/ui/animate";

type NavigationState = "start" | "complete" | "hide";

export function NavigationProgress() {
	const [state, setState] = useState<NavigationState>("hide");
	const [pulse, setPulse] = useState(false);
	const navigation = useNavigation();

	useEffect(() => {
		setState(navigation.state === "idle" ? "complete" : "start");
	}, [navigation.state]);

	const getConfig = () => {
		switch (state) {
			case "start":
				return { target: "90%", duration: 60, ease: "circOut" };
			case "complete":
				return { target: "100%", duration: 0.5, ease: "circIn" };
			default:
				return { hidden: true };
		}
	};

	const { target, duration, hidden, ease } = getConfig();

	return (
		<div
			className={cn("fixed top-0 z-[1000] flex h-[2px] w-full bg-transparent")}
		>
			<AnimatePresence mode="wait">
				{!hidden && (
					<Animate.div
						initial={{ width: 0 }}
						animate={{
							width: target,
							transition: { ease, duration },
						}}
						exit={{
							x: "100%",
							opacity: 0.7,
							transition: { duration: 0.7, ease: "easeIn" },
						}}
						onAnimationComplete={() => {
							setPulse(false);
							if (state === "complete") return setState("hide");
							if (state === "start") return setPulse(true);
						}}
						className={cn("h-full bg-primary", {
							"animate-pulse": pulse,
						})}
					></Animate.div>
				)}
			</AnimatePresence>
		</div>
	);
}
