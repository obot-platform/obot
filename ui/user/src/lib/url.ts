import { browser } from '$app/env';
import { goto as svelteGoTo } from '$app/navigation';
import { resolve as svelteResolve } from '$app/paths';
import { navigating, page, type ReadonlyURL } from '$app/state';
import type { InitSort } from './components/table/Table.svelte';

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

export async function setFilterUrlParams(filters: Record<string, string[]>) {
	const url = new URL(page.url.toString());
	for (const [key, values] of Object.entries(filters)) {
		if (values.length === 0) {
			url.searchParams.delete(key);
		} else {
			url.searchParams.set(key, values.join(','));
		}
	}
	return goto(url, { replace: true, reset: false, shallow: true });
}

export function getTableUrlParamsFilters() {
	const urlFilters: Record<string, string[]> = {};
	if (page.url.searchParams.size > 0) {
		page.url.searchParams.forEach((value, key) => {
			if (
				key === 'sort' ||
				key === 'sortDirection' ||
				key === 'query' ||
				key === 'from' ||
				key === 'view' ||
				key === 'new'
			)
				return;
			urlFilters[key] = value.split(',');
		});
	}
	return urlFilters;
}

export function getTableUrlParamsSort(defaultSort?: InitSort) {
	return page.url.searchParams.get('sort')
		? {
				property: page.url.searchParams.get('sort')!,
				order: (page.url.searchParams.get('sortDirection') as 'asc' | 'desc') || 'asc'
			}
		: defaultSort;
}

export function setSortUrlParams(property?: string, direction?: 'asc' | 'desc') {
	if (!property || !direction) {
		const url = new URL(page.url.toString());
		url.searchParams.delete('sort');
		url.searchParams.delete('sortDirection');
		goto(url, { reset: false, replace: true, shallow: true });
		return;
	}
	const url = new URL(page.url.toString());
	url.searchParams.set('sort', property);
	url.searchParams.set('sortDirection', direction);
	goto(url, { reset: false, replace: true, shallow: true });
}

export function clearUrlParams(params = Array.from(page.url.searchParams.keys())) {
	// Collect all keys first to avoid issues with modifying during iteration
	const url = new URL(page.url.toString());
	for (const key of params) {
		url.searchParams.delete(key);
	}
	goto(url, { reset: false, replace: true, shallow: true });
}

export function setSearchParamsToLocalStorage(pathname: string, searchParams: string) {
	if (browser) {
		localStorage.setItem(`page.searchParams.${pathname}`, searchParams);
	}
}

export function getSearchParamsFromLocalStorage(pathname: string): string | null {
	if (browser) {
		return localStorage.getItem(`page.searchParams.${pathname}`);
	}

	return null;
}

type NavigationOptions = Partial<Parameters<typeof svelteGoTo>[1]> & {
	replaceState?: boolean;
	noScroll?: boolean;
	keepFocus?: boolean;
};

export function goto(url: string | URL | ReadonlyURL, options: NavigationOptions = {}) {
	const routeToUse =
		typeof url === 'string' ? url : `/${url.pathname}${url.search ? `${url.search}` : ''}`;
	const resolved = resolve(routeToUse);
	const { replaceState, noScroll, keepFocus, ...rest } = options;
	return svelteGoTo(
		resolved as never,
		{
			...rest,
			...(replaceState === undefined ? {} : { replace: replaceState }),
			...(noScroll || keepFocus ? { reset: false } : {})
		} as Parameters<typeof svelteGoTo>[1]
	);
}

export function resolve(path: string, params?: Record<string, string | number>): string {
	return (
		svelteResolve as unknown as (path: string, params?: Record<string, string | number>) => string
	)(path, params);
}

export function replaceState(url: string | URL, state: object = {}) {
	return goto(url, { state, replace: true, reset: false, shallow: true });
}

export function setUrlParam(url: URL, key: string, value: string | null): void {
	if (value) {
		url.searchParams.set(key, value);
	} else {
		url.searchParams.delete(key);
	}
}

export function setUrlParamAndUpdateUrl(
	url: string | URL | ReadonlyURL,
	key: string,
	value?: string | null
): void {
	const newUrl = url instanceof URL ? new URL(url) : new URL(url.toString());
	setUrlParam(newUrl, key, value || null);
	goto(newUrl, { reset: false, replace: true });
}

export function camelToSnakeCase(str: string): string {
	return str.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`);
}

export function buildQueryString(
	filters: Record<string, string | number | boolean | string[] | undefined | null>
) {
	return Object.entries(filters)
		.filter(([_, value]) => value !== undefined && value !== null)
		.map(([key, value]) => {
			if (Array.isArray(value)) {
				// Join arrays with commas for multi-value parameters
				return `${camelToSnakeCase(key)}=${encodeURIComponent(value.join(','))}`;
			}
			return `${camelToSnakeCase(key)}=${typeof value === 'string' ? encodeURIComponent(value) : value}`;
		})
		.join('&');
}

export function isWebURL(value: string) {
	try {
		const url = new URL(value);
		return url.protocol === 'https:' || url.protocol === 'http:';
	} catch {
		return false;
	}
}
