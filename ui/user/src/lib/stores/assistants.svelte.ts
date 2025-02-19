import { getAssistant, listAssistants } from '$lib/services/chat/operations';
import { type Assistant } from '$lib/services/chat/types';
import context, { onInit } from '$lib/stores/context.svelte';

const defaultAssistant: Assistant = {
	id: '',
	icons: {}
};

const store = $state({
	items: [] as Assistant[],
	loaded: false,
	current: () => {
		const id = context.assistantID;
		return (
			store.items.find((assistant) => assistant.alias === id || assistant.id === id) ||
			defaultAssistant
		);
	},
	load: async () => {
		store.items = (await listAssistants()).items;
		store.loaded = true;
	}
});

onInit(() => {
	listAssistants().then((assistants) => {
		store.items = assistants.items;
		store.loaded = true;

		const currentID = context.assistantID;
		if (currentID) {
			const assistant = store.items.find(
				(assistant) => assistant.id === currentID || assistant.alias === currentID
			);
			if (assistant) {
				assistant.current = true;
			} else {
				getAssistant(currentID).then((assistant) => {
					assistant.current = true;
					store.items.push(assistant);
				});
			}
		}
	});
});

export default store;
