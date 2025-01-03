import { getVersion } from '$lib/services/chat/operations';
import { type Version } from '$lib/services/chat/types';

const version: {
	current: Version;
} = $state({
	current: {
		emailDomain: '',
		dockerSupported: false
	}
});

async function init() {
	version.current = await getVersion();
}

if (typeof window !== 'undefined') {
	init();
}

export default version;
