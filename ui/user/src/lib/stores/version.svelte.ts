import { getVersion } from '$lib/services/chat/operations';
import { type Version } from '$lib/services/chat/types';
import { onInit } from '$lib/stores/context.svelte';

const store = $state({
	current: {
		emailDomain: '',
		dockerSupported: false
	} as Version
});

onInit(async () => {
	store.current = await getVersion();
});

export default store;
