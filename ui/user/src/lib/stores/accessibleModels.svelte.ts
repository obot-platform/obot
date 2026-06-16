import { UserService, type Model } from '$lib/services';
import { SUPPORTED_PROVIDER_IDS } from '$lib/services/llm-gateway/types';

const SUPPORTED_MODEL_PROVIDER_IDS = new Set<string>(SUPPORTED_PROVIDER_IDS);

export function filterAccessibleModels(models: Model[]): Model[] {
	return models.filter((m) => m.active && SUPPORTED_MODEL_PROVIDER_IDS.has(m.modelProvider));
}

const store = $state<{
	current: Model[];
	loading: boolean;
	initialized: boolean;
	set: (models: Model[]) => void;
	initialize: (models?: Model[]) => Promise<void>;
	refresh: () => Promise<void>;
}>({
	current: [],
	loading: false,
	initialized: false,
	set: setModels,
	initialize,
	refresh
});

function setModels(models: Model[]) {
	store.current = filterAccessibleModels(models);
	store.initialized = true;
}

async function load() {
	store.loading = true;
	try {
		const all = await UserService.listModels();
		store.current = filterAccessibleModels(all);
	} catch {
		store.current = [];
	} finally {
		store.loading = false;
		store.initialized = true;
	}
}

async function initialize(models?: Model[]) {
	if (store.initialized) return;
	if (models) {
		setModels(models);
	} else {
		await load();
	}
}

async function refresh() {
	await load();
}

export default store;
