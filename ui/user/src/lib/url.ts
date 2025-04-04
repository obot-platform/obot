import { browser } from '$app/environment';
import { navigating } from '$app/state';

export function qIsSet(key: string): boolean {
	if (navigating?.to?.url.searchParams.has(key)) {
		return true;
	}
	return browser && new URL(window.location.href).searchParams.has(key);
}

export function q(key: string): string {
	if (navigating?.to?.url.searchParams.has(key)) {
		return navigating.to.url.searchParams.get(key) || '';
	}
	return browser ? new URL(window.location.href).searchParams.get(key) || '' : '';
}
