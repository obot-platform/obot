import { AdminService, type AuthProvider } from '$lib/services';
import { getStatus } from '$lib/services/llm-gateway/gateway';
import { writable, get } from 'svelte/store';

interface AdminConfig {
	// Whether the LLM gateway has at least one provider configured.
	modelProviderConfigured: boolean;
	// Whether modelProviderConfigured came from a ready gateway. While the gateway is starting
	// it can't say, and the store asks again until it can.
	modelProviderKnown: boolean;
	authProviderConfigured: boolean;
	authProviders: AuthProvider[];
	eulaAccepted: boolean;
	loading: boolean;
	lastFetched: number | null;
}

const createAdminConfigStore = () => {
	const { subscribe, set, update } = writable<AdminConfig>({
		modelProviderConfigured: false,
		modelProviderKnown: false,
		authProviderConfigured: false,
		authProviders: [],
		eulaAccepted: false,
		loading: false,
		lastFetched: null
	});

	let isInitialized = false;
	let retry: ReturnType<typeof setTimeout> | undefined;

	const fetchData = async (forceRefresh = false) => {
		const now = Date.now();
		const cacheAge = 5 * 60 * 1000; // 5 minutes cache

		// Return cached data if it's fresh and not forcing refresh
		if (!forceRefresh && isInitialized && cacheAge > 0) {
			const currentState = get({ subscribe });
			if (
				currentState.lastFetched &&
				currentState.modelProviderKnown &&
				now - currentState.lastFetched < cacheAge
			) {
				return;
			}
		}

		update((state) => ({ ...state, loading: true }));

		try {
			const [gatewayStatus, authProviders, eula] = await Promise.all([
				getStatus(),
				AdminService.listAuthProviders(),
				AdminService.getEula()
			]);

			// While the gateway is starting or reloading it can't say whether it has providers.
			// Keep the last known answer (or assume it has, rather than send an admin through
			// setup), and ask again shortly. The rest of the configuration is current either way.
			const previous = get({ subscribe });
			const known = gatewayStatus.ready;
			const modelProviderConfigured = known
				? gatewayStatus.hasProviders
				: previous.modelProviderKnown
					? previous.modelProviderConfigured
					: true;
			const authProviderConfigured = authProviders.some((provider) => provider.configured);
			const eulaAccepted = eula.accepted;

			set({
				modelProviderConfigured,
				modelProviderKnown: known,
				authProviderConfigured,
				authProviders,
				eulaAccepted,
				loading: false,
				lastFetched: now
			});

			isInitialized = true;
			clearTimeout(retry);
			if (!known) {
				retry = setTimeout(() => fetchData(true), 5000);
			}
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

	const setModelProviderConfigured = (modelProviderConfigured: boolean) => {
		update((state) => ({ ...state, modelProviderConfigured, modelProviderKnown: true }));
	};

	const updateEula = (eulaAccepted: boolean) => {
		update((state) => ({
			...state,
			eulaAccepted
		}));
	};

	return {
		subscribe,
		refresh,
		initialize,
		fetchData,
		updateAuthProviders,
		setModelProviderConfigured,
		updateEula
	};
};

export const adminConfigStore = createAdminConfigStore();
