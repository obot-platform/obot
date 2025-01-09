import { useState } from "react";

type Comparison<T> = "shallowish" | ((a: T, b: T) => boolean);

export function useWatcher<T>(
	current: T,
	comparison: Comparison<T> = Object.is
) {
	const [previous, setPrevious] = useState<[T] | null>(null);

	const compare = getCompareFn(comparison);

	const hasChanged = !previous || !compare(current, previous[0]);
	const isFirst = previous === null;
	const update = () => setPrevious([current]);

	return {
		previous: previous?.[0],
		changed: hasChanged,
		isFirst,
		update,
	} as const;
}

function getCompareFn<T>(comparison: Comparison<T>): (a: T, b: T) => boolean {
	switch (comparison) {
		case "shallowish":
			return shallowishCompare;
		default:
			return comparison;
	}
}

function shallowishCompare<T>(a: T, b: T) {
	if (a === b) return true;

	if (Array.isArray(a) && Array.isArray(b)) {
		return a.every((value, index) => value === b[index]);
	}

	if (typeof a === "object" && typeof b === "object") {
		if (a === null || b === null) return false;

		return Object.keys(a).every(
			(key) => a[key as keyof T] === b[key as keyof T]
		);
	}

	return false;
}
