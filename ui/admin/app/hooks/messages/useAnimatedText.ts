import { animate, useMotionValue } from "motion/react";
import { useEffect, useState } from "react";

export function useAnimatedText(text: string, disabled?: boolean) {
	const animatedCursor = useMotionValue(0);
	const [cursor, setCursor] = useState(0);
	const [prev, setPrev] = useState(text);
	const [isSameText, setIsSameText] = useState(true);
	const [isAnimating, setIsAnimating] = useState(false);

	if (prev !== text) {
		setPrev(text);

		const textStartsWithPrev = text.startsWith(prev);
		setIsSameText(textStartsWithPrev);
		if (!textStartsWithPrev) {
			animatedCursor.set(cursor);
		}
	}

	useEffect(() => {
		if (!isSameText) {
			animatedCursor.jump(0);
		}

		setIsAnimating(true);

		const controls = animate(animatedCursor, text.length, {
			duration: 0.5,
			ease: "linear",
			onUpdate(latest) {
				setCursor(Math.floor(latest));
			},
			onComplete() {
				setIsAnimating(false);
			},
		});

		return () => controls.stop();
	}, [animatedCursor, isSameText, text.length]);

	return [disabled ? text : text.slice(0, cursor), isAnimating] as const;
}
