import { type License } from '$lib/services/admin/types';

export const emptyLicense: License = {
	entitlements: {
		community: false,
		enterprise: false,
		cloud: false,
		enterpriseAuthProviders: false,
		enterpriseModelProviders: false,
		users: null,
		devices: null,
		hostedMcpServers: null,
		memoryMiB: null,
		cpuMillicores: null,
		auditLogRetentionDays: null
	},
	usage: {
		users: 0,
		devices: 0,
		hostedMcpServers: 0,
		memoryMiB: 0,
		cpuMillicores: 0
	},
	licenseKey: '',
	locked: false
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
