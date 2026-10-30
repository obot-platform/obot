import { type License } from '$lib/services/admin/types';

export const emptyLicense: License = {
	licenseKey: '',
	source: '',
	locked: false,
	enterprise: false,
	entitlements: {
		community: false,
		enterprise: false,
		cloud: false,
		enterpriseAuthProviders: false,
		enterpriseModelProviders: false,
		users: null,
		devices: null,
		memoryMiB: null,
		cpuMillicores: null,
		auditLogRetentionDays: null
	}
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
