import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch }) => {
    const version = await ChatService.getVersion({ fetch });
    if (!version.nanobotEnabled) {
        throw redirect(302, '/');
    }
};
