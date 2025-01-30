import type { Context } from '$lib/services/chat/types';
import { get, writable } from 'svelte/store';

const store = writable<Context>({
	assistantID: '',
	projectID: 'default',
	valid: false
});

function init(cb: (context: Context) => Promise<void> | void) {
	if (typeof window === 'undefined') {
		return;
	}
	let initialed = false;
	store.subscribe(async (value) => {
		if (initialed || !value.valid) {
			return;
		}

		setTimeout(() => {
			const ret = cb(value);
			if (ret instanceof Promise) {
				ret.then(() => {
					initialed = true;
				});
			} else {
				initialed = true;
			}
		});
	});
}

function setContext(context: Context) {
	store.set(context);
}

function getContext() {
	return get(store);
}

export default {
	setContext,
	getContext,
	init
};
