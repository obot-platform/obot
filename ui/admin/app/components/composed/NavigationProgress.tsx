import { useEffect, useState } from "react";
import { useNavigation } from "react-router";

import { cn } from "~/lib/utils";

import { Animate, AnimatePresence } from "~/components/ui/animate";

type NavigationState = "start" | "complete" | "hide";

export function NavigationProgress() {
	const [state, setState] = useState<NavigationState>("hide");
	const navigation = useNavigation();

	useEffect(() => {
		setState(navigation.state === "idle" ? "complete" : "start");
	}, [navigation.state]);

	const getConfig = () => {
		if (state === "hide") {
			return { duration: 1, hidden: true };
		}

		if (state === "start") {
			return { target: "90%", duration: 60 };
		}

		return { target: "100%", duration: 0.3, complete: true };
	};

	const { target, duration, hidden, complete } = getConfig();

	return (
		<div className="fixed top-0 z-[1000] h-[2px] w-full bg-transparent">
			<AnimatePresence>
				{!hidden && (
					<Animate.div
						initial={{ width: 0 }}
						animate={{ width: target }}
						transition={{ ease: "circOut", duration }}
						exit={{ opacity: 0 }}
						onAnimationComplete={() => {
							if (state === "complete") return setState("hide");
						}}
						className={cn("h-full bg-primary", {
							"animate-pulse": !complete,
						})}
					></Animate.div>
				)}
			</AnimatePresence>
		</div>
	);
}
