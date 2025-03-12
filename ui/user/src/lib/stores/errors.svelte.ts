const store = $state({
	items: [] as Error[],
	append
});

export function isError(value: unknown): value is Error {
	return (
		value instanceof Error ||
		(typeof value === 'object' &&
			value !== null &&
			'message' in value &&
			typeof value.message === 'string')
	);
}

function append(e: unknown, duration?: number) {
	const err = isError(e) ? e : new Error(String(e));
	store.items.push(err);

	if (duration) {
		setTimeout(() => {
			store.items = store.items.filter((x) => x !== err);
		}, duration);
	}
}

export default store;
