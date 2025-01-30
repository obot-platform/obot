import { getProfile } from '$lib/services/chat/operations';
import { type Profile } from '$lib/services/chat/types';

const store = $state({
	current: {
		email: '',
		iconURL: '',
		role: 0
	} as Profile
});

async function init() {
	try {
		store.current = await getProfile();
	} catch (e) {
		if (e instanceof Error && (e.message.startsWith('403') || e.message.startsWith('401'))) {
			store.current = {
				email: '',
				iconURL: '',
				role: 0,
				unauthorized: true
			};
		} else {
			setTimeout(init, 5000);
		}
	}
}

if (typeof window !== 'undefined') {
	init().then(() => console.log('Profile initialized'));
}

export default store;
