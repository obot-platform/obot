import { browser } from '$app/environment';
import { getVersion } from '$lib/services/chat/operations';
import { type Version } from '$lib/services/chat/types';

const store = $state<{ current: Version }>({
	current: {}
});

async function init() {
	try {
		store.current = await getVersion();
	} catch {
		setTimeout(init, 5000);
	}
}

if (browser) {
	init().then(() => console.log('Version initialized'));
}

export default store;
