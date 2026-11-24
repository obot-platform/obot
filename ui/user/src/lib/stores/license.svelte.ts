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
		auditLogRetentionDays: null,
		auditLogStorageMiB: null,
		networkEgressMiB: null
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

// A license file in effect leaves licenseKey empty, so check both to tell a license is installed.
export function hasLicense(license: License): boolean {
	return Boolean(license.licenseKey) || Boolean(license.licenseFile);
}

// Registering for a Community license contacts Obot's services, so the server refuses it for an air-gapped
// installation.
export function canRegisterCommunity(license: License): boolean {
	return !license.airgap;
}

export default store;
