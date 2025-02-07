import { mutate } from "swr";

export type KeyObj<TParams> = {
	key: string;
	params: TParams;
};

/**
 * @description - Recusively compares two objects (`o1` and `o2`), returning true if all values from `o1` are equal to the same values for `o2`. Properties that are `undefined` in `o1` are ignored
 * @example
 * recursiveCompareKeys({ a: 1, b: 2 }, { a: 1, b: 2, c: 3 }) // true
 * recursiveCompareKeys({ a: 1, b: 2 }, { a: 1, b: 3 }) // false
 * recursiveCompareKeys({ a: 1, b: 2 }, { a: 1 }) // true
 * recursiveCompareKeys({ a: 1, b: undefined }, { a: 1, b: 2 }) // true
 * recursiveCompareKeys({ a: 1, b: 2 }, { a: 1, b: null }) // false
 */
export const recursiveCompareKeys = (
	o1: object | null,
	o2: object | null
): boolean => {
	// if either is null, they are equal if they are both null
	if (o1 === null || o2 === null) return o1 === o2;

	return Object.entries(o1).every(([key, value]) => {
		// skip undefined values on o1
		if (value === undefined) return true;

		const o2Value: unknown = key in o2 ? o2[key as keyof typeof o2] : undefined;

		// if o2Value is undefined, the keys are not equal
		if (o2Value === undefined) return false;

		// shallowly compare primitive values
		if (typeof value !== "object" || typeof o2Value !== "object") {
			return value === o2Value;
		}

		// recursively compare nested objects
		return recursiveCompareKeys(value, o2Value);
	});
};

export const revalidateObject = (keyObj: KeyObj<object>) => {
	mutate((cacheKey) => {
		if (typeof cacheKey !== "object") return false;

		return recursiveCompareKeys(keyObj, cacheKey);
	});
};
