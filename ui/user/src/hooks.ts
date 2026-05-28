import type { Reroute } from '@sveltejs/kit';

const ADMIN_DEVICES_PREFIX = '/admin/devices';

export const reroute: Reroute = ({ url }) => {
	const { pathname } = url;

	if (pathname === ADMIN_DEVICES_PREFIX || pathname.startsWith(`${ADMIN_DEVICES_PREFIX}/`)) {
		return pathname.replace(ADMIN_DEVICES_PREFIX, '/devices');
	}
};
