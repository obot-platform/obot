import { ChatService, Group, type AccessControlRule } from '$lib/services';
import { profile } from '.';
import { writable, get } from 'svelte/store';

interface WorkspaceConfig {
	rules: AccessControlRule[];
	loading: boolean;
	lastFetched: number | null;
	id: string;
}

const createWorkspaceStore = () => {
	const { subscribe, set, update } = writable<WorkspaceConfig>({
		rules: [],
		loading: false,
		lastFetched: null,
		id: ''
	});

	let isInitialized = false;

	const requiresRefresh = (now = Date.now(), forceRefresh = false) => {
		const cacheAge = 5 * 60 * 1000; // 5 minutes cache

		if (!forceRefresh && isInitialized && cacheAge > 0) {
			const currentState = get({ subscribe });
			if (currentState.lastFetched && now - currentState.lastFetched < cacheAge) {
				return false;
			}
		}
		return true;
	};
	const fetchData = async (forceRefresh = false) => {
		const now = Date.now();
		if (!requiresRefresh(now, forceRefresh)) {
			return;
		}

		update((state) => ({ ...state, loading: true }));

		try {
			const workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id);
			const rules = profile.current?.groups.includes(Group.POWERUSER_PLUS)
				? await ChatService.listWorkspaceAccessControlRules(workspaceId)
				: [];
			set({
				rules,
				loading: false,
				lastFetched: now,
				id: workspaceId
			});

			isInitialized = true;
		} catch (error) {
			console.error('Failed to fetch workspace config:', error);
			update((state) => ({ ...state, loading: false }));
		}
	};

	const refresh = () => fetchData(true);

	const initialize = () => {
		if (!isInitialized) {
			fetchData();
		}
	};

	const listRules = async () => {
		const now = Date.now();
		if (!requiresRefresh(now)) {
			return get({ subscribe }).rules;
		}

		const workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id);
		const rules = await ChatService.listWorkspaceAccessControlRules(workspaceId);
		update((state) => ({ ...state, rules, loading: false, lastFetched: now }));

		return rules;
	};

	return {
		subscribe,
		refresh,
		initialize,
		fetchData,
		listRules
	};
};

export const workspaceStore = createWorkspaceStore();
