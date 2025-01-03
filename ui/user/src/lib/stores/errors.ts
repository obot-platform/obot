import { writable } from 'svelte/store';

const store = writable([] as Error[]);

function append(e: Error) {
	store.update((v) => {
		return [...v, e];
	});
}

function remove(index: number) {
	store.update((v) => {
		return v.filter((v, i) => i !== index);
	});
}

export default {
	subscribe: store.subscribe,
	append,
	remove
};
