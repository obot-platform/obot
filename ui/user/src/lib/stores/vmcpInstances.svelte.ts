import { browser } from '$app/environment';
import { UserService, type VMCPInstance } from '$lib/services';
import profile from './profile.svelte';

interface VMCPInstancesState {
	items: VMCPInstance[];
	loading: boolean;
}

let refreshPromise: Promise<void> | undefined;
let watchingConsumers = 0;
let isUserActive = true;

const store = $state<{
	current: VMCPInstancesState;
	refresh: () => Promise<void>;
	upsert: (instance: VMCPInstance) => void;
	remove: (id: string) => void;
	startWatching: () => () => void;
}>({
	current: {
		items: [],
		loading: false
	},
	refresh,
	upsert,
	remove,
	startWatching
});

function canListVMCPInstances() {
	return (
		profile.current.loaded === true &&
		!profile.current.unauthorized &&
		!profile.current.expired &&
		!!profile.current.id
	);
}

async function refresh() {
	if (!canListVMCPInstances()) {
		store.current = {
			items: [],
			loading: false
		};
		return;
	}

	if (refreshPromise) {
		return refreshPromise;
	}

	const profileID = profile.current.id;

	store.current = {
		...store.current,
		loading: true
	};

	refreshPromise = (async () => {
		try {
			const items = await UserService.listVMCPInstances({
				dontLogErrors: true
			});
			if (!canListVMCPInstances() || profile.current.id !== profileID) {
				store.current = {
					items: [],
					loading: false
				};
				return;
			}
			store.current = {
				items,
				loading: false
			};
		} catch {
			store.current = {
				items: [],
				loading: false
			};
		} finally {
			refreshPromise = undefined;
		}
	})();

	return refreshPromise;
}

function upsert(instance: VMCPInstance) {
	const items = store.current.items;
	const index = items.findIndex((candidate) => candidate.id === instance.id);
	store.current = {
		...store.current,
		items:
			index >= 0
				? [...items.slice(0, index), instance, ...items.slice(index + 1)]
				: [...items, instance]
	};
}

function remove(id: string) {
	store.current = {
		...store.current,
		items: store.current.items.filter((item) => item.id !== id)
	};
}

function handleUserAway() {
	if (!isUserActive) return;
	isUserActive = false;
}

function handleUserReturned() {
	if (isUserActive) return;
	isUserActive = true;
	void refresh();
}

function handleVisibilityChange() {
	if (document.hidden) {
		handleUserAway();
	} else {
		handleUserReturned();
	}
}

function startWatching() {
	watchingConsumers++;
	void refresh();

	if (browser && watchingConsumers === 1) {
		isUserActive = !document.hidden;
		document.addEventListener('visibilitychange', handleVisibilityChange);
		window.addEventListener('blur', handleUserAway);
		window.addEventListener('focus', handleUserReturned);
	}

	return () => {
		watchingConsumers = Math.max(0, watchingConsumers - 1);
		if (watchingConsumers === 0 && browser) {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
			window.removeEventListener('blur', handleUserAway);
			window.removeEventListener('focus', handleUserReturned);
			isUserActive = true;
		}
	};
}

export default store;
