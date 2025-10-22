import { AdminService, type AuthProvider, type ModelProvider } from '$lib/services';
import { writable, get } from 'svelte/store';

interface AdminConfig {
	modelProviderConfigured: boolean;
	authProviderConfigured: boolean;
	loading: boolean;
	lastFetched: number | null;
}

const createAdminConfigStore = () => {
	const { subscribe, set, update } = writable<AdminConfig>({
		modelProviderConfigured: false,
		authProviderConfigured: false,
		loading: false,
		lastFetched: null
	});

	let isInitialized = false;

	const fetchData = async (forceRefresh = false) => {
		const now = Date.now();
		const cacheAge = 5 * 60 * 1000; // 5 minutes cache

		// Return cached data if it's fresh and not forcing refresh
		if (!forceRefresh && isInitialized && cacheAge > 0) {
			const currentState = get({ subscribe });
			if (currentState.lastFetched && now - currentState.lastFetched < cacheAge) {
				return;
			}
		}

		update((state) => ({ ...state, loading: true }));

		try {
			const [modelProviders, authProviders] = await Promise.all([
				AdminService.listModelProviders(),
				AdminService.listAuthProviders()
			]);

			console.log('modelProviders', modelProviders);
			console.log('authProviders', authProviders);
			const modelProviderConfigured = modelProviders.some((provider) => provider.configured);
			const authProviderConfigured = authProviders.some((provider) => provider.configured);

			set({
				modelProviderConfigured,
				authProviderConfigured,
				loading: false,
				lastFetched: now
			});

			isInitialized = true;
		} catch (error) {
			console.error('Failed to fetch admin config:', error);
			update((state) => ({ ...state, loading: false }));
		}
	};

	const refresh = () => fetchData(true);

	const initialize = () => {
		if (!isInitialized) {
			fetchData();
		}
	};

	const updateAuthProviders = (authProviders: AuthProvider[]) => {
		update((state) => ({
			...state,
			authProviders,
			authProviderConfigured: authProviders.some((provider) => provider.configured)
		}));
	};

	const updateModelProviders = (modelProviders: ModelProvider[]) => {
		update((state) => ({
			...state,
			modelProviders,
			modelProviderConfigured: modelProviders.some((provider) => provider.configured)
		}));
	};

	return {
		subscribe,
		refresh,
		initialize,
		fetchData,
		updateAuthProviders,
		updateModelProviders
	};
};

export const adminConfigStore = createAdminConfigStore();
