import { getVersion } from '$lib/services/chat/operations';
import { type Version } from '$lib/services/chat/types';
import context from '$lib/stores/context';

const store = $state({
	current: {
		emailDomain: '',
		dockerSupported: false
	} as Version
});

context.init(async () => {
	store.current = await getVersion();
});

export default store;
