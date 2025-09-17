import { BOOTSTRAP_USER_ID } from '$lib/constants';
import { getProfile } from '$lib/services/chat/operations';
import { type Profile } from '$lib/services/chat/types';

const store = $state({
	current: {
		id: '',
		username: '',
		email: '',
		iconURL: '',
		role: 0,
		groups: []
	} as Profile
});

async function init() {
	try {
		store.current = await getProfile();
		if (store.current.username === BOOTSTRAP_USER_ID) {
			store.current.displayName = 'Bootstrap';
		}
	} catch (e) {
		if (e instanceof Error && (e.message.startsWith('403') || e.message.startsWith('401'))) {
			store.current = {
				id: '',
				email: '',
				iconURL: '',
				role: 0,
				groups: [],
				unauthorized: true,
				username: ''
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
