import { type License } from '$lib/services/admin/types';

const emptyLicense: License = {
	licenseKey: '',
	source: '',
	locked: false,
	enterprise: false,
	entitlements: null
};
const store = $state<{ current: License; initialize: (license?: License) => void }>({
	current: emptyLicense,
	initialize
});

function initialize(license?: License) {
	if (license) {
		store.current = license;
	} else {
		store.current = emptyLicense;
	}
}

export default store;
