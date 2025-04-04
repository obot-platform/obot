import { writable } from 'svelte/store';

export interface SuccessMessage {
	message: string;
	id: number;
}

export const success = (() => {
	const { subscribe, update } = writable<SuccessMessage[]>([]);
	let nextId = 0;

	return {
		subscribe,
		add: (message: string) => {
			const id = nextId++;
			update((messages) => [...messages, { message, id }]);
			setTimeout(() => {
				update((messages) => messages.filter((m) => m.id !== id));
			}, 5000);
		},
		remove: (id: number) => {
			update((messages) => messages.filter((m) => m.id !== id));
		}
	};
})();
