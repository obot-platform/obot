import { AdminService, type ImagePullSecret, type ImagePullSecretCapability } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [capability, imagePullSecrets] = await Promise.all([
		AdminService.getImagePullSecretCapability({ fetch }),
		AdminService.listImagePullSecrets({ fetch })
	]);

	return {
		capability: capability as ImagePullSecretCapability,
		imagePullSecrets: imagePullSecrets as ImagePullSecret[]
	};
};
