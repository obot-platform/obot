import { type Profile } from '$lib/services/chat/types';

const store = $state({
	current: {
		id: '',
		username: '',
		email: '',
		iconURL: '',
		role: 0,
		effectiveRole: 0,
		groups: []
	} as Profile,
	initialize
});

function initialize(profile?: Profile) {
	if (profile) {
		store.current = profile;
		if (profile.isBootstrapUser?.()) {
			store.current.displayName = 'Bootstrap';
		}
	} else {
		store.current = {
			id: '',
			email: '',
			iconURL: '',
			role: 0,
			effectiveRole: 0,
			groups: [],
			unauthorized: true,
			username: ''
		};
	}
}

export default store;
