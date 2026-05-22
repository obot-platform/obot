import type { ImagePullSecret } from './types';

export function canTest(secret: ImagePullSecret) {
	return !secret.manifest.enabled || !secret.status?.lastError;
}
