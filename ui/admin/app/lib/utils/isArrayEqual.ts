export function isArrayEqual<T>(
	a: T[],
	b: T[],
	comparison: (a: T, b: T) => boolean = Object.is
) {
	return (
		a.length === b.length &&
		a.every((value, index) => comparison(value, b[index]))
	);
}
