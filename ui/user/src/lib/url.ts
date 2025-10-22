import { browser } from '$app/environment';
import { replaceState } from '$app/navigation';
import { navigating, page } from '$app/state';
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

export function setFilterUrlParams(property: string, values: string[]) {
	if (values.length === 0) {
		page.url.searchParams.delete(property);
	} else {
		page.url.searchParams.set(property, values.join(','));
	}

	replaceState(page.url, {});
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
				key === 'view'
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
		page.url.searchParams.delete('sort');
		page.url.searchParams.delete('sortDirection');
		replaceState(page.url, {});
		return;
	}
	page.url.searchParams.set('sort', property);
	page.url.searchParams.set('sortDirection', direction);
	replaceState(page.url, {});
}

export function clearUrlParams() {
	// Collect all keys first to avoid issues with modifying during iteration
	const keysToDelete = Array.from(page.url.searchParams.keys());
	for (const key of keysToDelete) {
		page.url.searchParams.delete(key);
	}
	replaceState(page.url, {});
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
