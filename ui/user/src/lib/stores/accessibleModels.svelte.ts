import { CommonModelProviderIds } from '$lib/constants';
import { UserService, type Model } from '$lib/services';

// Every model is served through the LLM gateway, so only its models are callable.
export function filterAccessibleModels(models: Model[]): Model[] {
	return models.filter((m) => m.active && m.modelProvider === CommonModelProviderIds.LLM_GATEWAY);
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
