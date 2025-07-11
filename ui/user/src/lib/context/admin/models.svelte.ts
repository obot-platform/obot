import type { Model } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('admin-models');

export interface AdminModelsContext {
	items: Model[];
}

export function getAdminModels() {
	if (!hasContext(Key)) {
		throw new Error('Admin models not initialized');
	}
	return getContext<AdminModelsContext>(Key);
}

export function initModels(models: Model[]) {
	const data = $state<AdminModelsContext>({ items: models });
	setContext(Key, data);
}
