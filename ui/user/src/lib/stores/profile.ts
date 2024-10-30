import { writable } from 'svelte/store';
import { ChatService, type Profile } from '$lib/services';
import { storeWithInit } from './storeinit';

const store = writable<Profile>({
	email: '',
	iconURL: '',
	role: 0
});

export default storeWithInit(store, async () => {
	store.set(await ChatService.getProfile());
});
