import { page } from '$app/stores';
import type { Assistant } from '$lib/services';
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
	return currentAssistants.find((value) => value.current) ?? def;
}

function init() {
	const p = get(page);
	const a = get(assistants);
	if (p && a.length > 0) {
		const selectedName = p.params?.agent ?? '';
		store.set(assignSelected(a, selectedName));
	}
}

if (typeof window !== 'undefined') {
	page.subscribe(init);
	assistants.subscribe(init);
}

export default store;
