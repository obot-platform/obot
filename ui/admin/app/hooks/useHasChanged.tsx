import { useState } from "react";

type Comparison<T> = "shallowish" | ((a: T, b: T) => boolean);

type Config<T> = {
	comparison?: Comparison<T>;
	runOnMount?: boolean;
};

export function useHasChanged<T>(current: T, config: Config<T> = {}) {
	const { comparison = Object.is, runOnMount = false } = config;
	const compare = getCompareFn(comparison);

	const [previous, setPrevious] = useState<[T] | null>(
		runOnMount ? null : [current]
	);

	const hasChanged = !previous || !compare(current, previous[0]);
	if (hasChanged) setPrevious([current]);

	return [hasChanged, previous?.[0]] as const;
}

function getCompareFn<T>(comparison: Comparison<T>): (a: T, b: T) => boolean {
	if (typeof comparison === "function") return comparison;

	if (comparison === "shallowish") return shallowishCompare;

	return Object.is;
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
