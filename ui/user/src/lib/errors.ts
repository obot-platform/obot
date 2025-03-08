import { error, redirect } from '@sveltejs/kit';
import type { Profile } from '$lib/services';

export function handleRouteError(e: unknown, path: string, profile?: Profile) {
    if (!(e instanceof Error)) {
      throw new Error('Unknown error occurred');
    }
  
    if (e.message?.includes('403') || e.message?.includes("forbidden")) {
      if (profile?.role === 0) {
        throw redirect(303, `/?rd=${path}`);
      }
      throw error(403, e.message);
    }
  
    if (e.message?.includes('404') || e.message?.includes("not found")) {
      throw error(404, e.message);
    }
  
    throw error(500, e.message);
  }