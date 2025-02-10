import { useCallback, useEffect, useRef, useState } from "react";

/** ensure that fn is properly memoized */
export const useDebounce = <TParams extends unknown[]>(
	fn: (...args: TParams) => void,
	delay: number
) => {
	const timerRef = useRef<NodeJS.Timeout>();
	const effectiveDelay = import.meta.env.VITEST ? 5 : delay;

	const debouncedFn = useCallback(
		(...args: TParams) => {
			clearTimeout(timerRef.current);
			timerRef.current = setTimeout(() => fn(...args), effectiveDelay);
		},
		[effectiveDelay, fn]
	);

	useEffect(() => {
		return () => clearTimeout(timerRef.current);
	}, []);

	return debouncedFn;
};

export const useDebouncedValue = <T>(value: T, delay: number) => {
	const [debouncedValue, setDebouncedValue] = useState(value);
	const effectiveDelay = import.meta.env.VITEST ? 0 : delay;

	useEffect(() => {
		const timer = setTimeout(() => setDebouncedValue(value), effectiveDelay);

		return () => clearTimeout(timer);
	}, [value, effectiveDelay]);

	return debouncedValue;
};
