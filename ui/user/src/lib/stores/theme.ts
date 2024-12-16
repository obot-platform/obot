import { writable } from 'svelte/store';

export type Theme = 'light' | 'dark' | 'system';
export const DefaultTheme: Theme = 'system';

const store = writable(DefaultTheme as Theme);

function init() {
	const initialTheme = localStorage.getItem('theme') as Theme | null;
	if (initialTheme) {
		store.set(initialTheme);
	}

	store.subscribe((value) => {
		if (value === 'system' && localStorage.getItem('theme') === null) {
			// no point in setting it
			return;
		}
		localStorage.setItem('theme', value);
	});
}

if (typeof window !== 'undefined') {
	init();
}

export default store;
