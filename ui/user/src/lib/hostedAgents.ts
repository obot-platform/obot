import type { Version } from '$lib/services/user/types';
import { redirect } from '@sveltejs/kit';

export function requireHostedAgentsEnabled(version: Version | undefined, fallback: string): void {
	if (version?.hostedAgentsEnabled !== true) {
		throw redirect(302, fallback);
	}
}
