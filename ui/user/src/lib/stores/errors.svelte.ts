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

function append(e: unknown) {
	if (isError(e)) {
		store.items.push(e);
	} else {
		store.items.push(new Error(String(e)));
	}
}

export default store;
