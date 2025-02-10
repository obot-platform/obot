import { AnimationProps } from "motion/react";

type Direction = "up" | "down" | "left" | "right";

type Config<T extends string | number> = T | { in: T; out: T };

export type SlideInOutProps = {
	direction?: Config<Direction>;
	translatePercent?: Config<number>;
	disabled?: boolean;
};

export const useSlideInOut = ({
	direction = "left",
	translatePercent = 100,
	disabled = false,
}: SlideInOutProps) => {
	if (disabled) return {};

	const { in: inDirection, out: outDirection } = valuesFromConfig(direction);

	const { in: inTranslate, out: outTranslate } =
		valuesFromConfig(translatePercent);

	const inX = getX(inDirection, "in");
	const inY = getY(inDirection, "in");

	const outX = getX(outDirection, "out");
	const outY = getY(outDirection, "out");

	function getY(direction: Direction, inOut: "in" | "out") {
		if (direction === "up") {
			return getTranslate(-1 * (inOut === "in" ? inTranslate : outTranslate));
		} else if (direction === "down") {
			return getTranslate(inOut === "in" ? inTranslate : outTranslate);
		}
		return 0;
	}

	function getX(direction: Direction, inOut: "in" | "out") {
		if (direction === "left") {
			return getTranslate(-1 * (inOut === "in" ? inTranslate : outTranslate));
		} else if (direction === "right") {
			return getTranslate(inOut === "in" ? inTranslate : outTranslate);
		}

		return 0;
	}

	function getTranslate(translate: number) {
		return translate.toString() + "%";
	}

	function valuesFromConfig<T extends string | number>(config: Config<T>) {
		if (typeof config === "object") {
			return config;
		}

		return { in: config, out: config };
	}

	return {
		initial: { opacity: 0, x: inX, y: inY },
		animate: { opacity: 1, x: 0, y: 0 },
		exit: { opacity: 0, x: outX, y: outY },
	} as AnimationProps;
};
