import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	type MDMConfiguration,
	type MDMDevice,
	type MDMEnrollmentKey
} from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { profile } = await parent();
	const id = Number(params.id);
	let configuration: MDMConfiguration | undefined;
	let enrollmentKeys: MDMEnrollmentKey[] = [];
	let devices: MDMDevice[] = [];

	try {
		[configuration, enrollmentKeys, devices] = await Promise.all([
			AdminService.getMDMConfiguration(id, { fetch }),
			AdminService.listMDMEnrollmentKeys(id, { fetch }),
			AdminService.listMDMDevices(id, { fetch })
		]);
	} catch (err) {
		handleRouteError(err, `/admin/mdm-configurations/${params.id}`, profile);
	}

	return { configuration, enrollmentKeys, devices };
};
