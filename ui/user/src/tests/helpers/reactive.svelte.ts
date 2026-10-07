/** Wrap test fixtures in Svelte state when a component binds to their properties. */
export function reactive<T>(value: T): T {
	const state = $state(value);
	return state;
}
