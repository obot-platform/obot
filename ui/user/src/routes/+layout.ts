import { building } from '$app/environment';

export const prerender = 'auto';
export const ssr = !building;
