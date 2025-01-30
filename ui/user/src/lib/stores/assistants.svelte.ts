import { getAssistant, listAssistants } from '$lib/services/chat/operations';
import { type Assistant } from '$lib/services/chat/types';
import context from '$lib/stores/context';

const defaultAssistant: Assistant = {
	id: '',
	icons: {}
};

const store = $state({
	items: [] as Assistant[],
	loaded: false,
	current: () => {
		const id = context.getContext().assistantID;
		return store.items.find((assistant) => assistant.id === id) || defaultAssistant;
	},
	load: async () => {
		store.items = (await listAssistants()).items;
		store.loaded = true;
	}
});

context.init(() => {
	listAssistants().then((assistants) => {
		store.items = assistants.items;
		store.loaded = true;

		const currentID = context.getContext().assistantID;
		if (currentID) {
			const assistant = store.items.find((assistant) => assistant.id === currentID);
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
