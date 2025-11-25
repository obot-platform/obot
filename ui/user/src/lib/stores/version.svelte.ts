import { type Version } from '$lib/services/chat/types';

const store = $state<{ current: Version; initialize: (version?: Version) => void }>({
	current: {},
	initialize
});

function initialize(version?: Version) {
	if (version) {
		store.current = version;
	} else {
		store.current = {};
	}
}

export default store;
