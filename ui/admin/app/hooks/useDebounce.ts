import { useCallback, useEffect, useRef, useState } from "react";

/** ensure that fn is properly memoized */
export const useDebounce = <TParams extends unknown[]>(
    fn: (...args: TParams) => void,
    delay: number
) => {
    const timerRef = useRef<NodeJS.Timeout>();

    const debouncedFn = useCallback(
        (...args: TParams) => {
            clearTimeout(timerRef.current);
            timerRef.current = setTimeout(() => fn(...args), delay);
        },
        [delay, fn]
    );

    useEffect(() => {
        return () => clearTimeout(timerRef.current);
    }, []);

    return debouncedFn;
};

export const useDebouncedValue = <T>(value: T, delay: number) => {
    const [debouncedValue, setDebouncedValue] = useState(value);

    useEffect(() => {
        const timer = setTimeout(() => setDebouncedValue(value), delay);

        return () => clearTimeout(timer);
    }, [value, delay]);

    return debouncedValue;
};
