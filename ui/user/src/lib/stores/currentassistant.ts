import { page } from '$app/stores';
import { type Assistant, ChatService } from '$lib/services';
import assistants from './assistants';
import { get, writable } from 'svelte/store';

const def: Assistant = {
	id: '',
	icons: {},
	current: false
};

const store = writable<Assistant>(def);

function assignSelected(currentAssistants: Assistant[], selectedName: string): Assistant {
	let changed = false;
	for (let i = 0; i < currentAssistants.length; i++) {
		const assistant = currentAssistants[i];
		const isCurrent = selectedName !== '' && assistant.id === selectedName;
		if (assistant.current != isCurrent) {
			assistant.current = isCurrent;
			changed = true;
		}
	}
	if (changed) {
		assistants.set(currentAssistants);
	}
	const res = currentAssistants.find((value) => value.current);
	if (!res && selectedName) {
		ChatService.getAssistant(selectedName)
			.then((assistant) => {
				if (assistant) {
					assistant.current = true;
					store.set(assistant);
				}
			})
			.catch((error) => {
				if (String(error).includes('404')) {
					window.location.href = '/';
				}
			});
		return def;
	}
	return res ?? def;
}

function init() {
	const p = get(page);
	const a = get(assistants);
	if (p && a.length > 0) {
		const selectedName = p.params?.agent ?? '';
		store.set(assignSelected(a, selectedName));
	} else if (p.params?.agent) {
		store.set(assignSelected(a, p.params.agent));
	}
}

if (typeof window !== 'undefined') {
	page.subscribe(init);
	assistants.subscribe(init);
}

export default store;
