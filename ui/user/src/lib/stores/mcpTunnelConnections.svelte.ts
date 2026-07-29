import { UserService, type TunnelConnection } from '$lib/services';
import profile from './profile.svelte';

interface MCPTunnelConnectionsState {
	connections: TunnelConnection[] | undefined;
	loading: boolean;
}

const POLL_INTERVAL_MS = 5000;

let pollingConsumers = 0;
let pollingInterval: ReturnType<typeof setInterval> | undefined;
let refreshPromise: Promise<void> | undefined;

const store = $state<{
	current: MCPTunnelConnectionsState;
	refresh: () => Promise<void>;
	startPolling: () => () => void;
}>({
	current: {
		connections: undefined,
		loading: false
	},
	refresh,
	startPolling
});

function canListTunnelConnections() {
	return (
		profile.current.loaded === true &&
		!profile.current.unauthorized &&
		!profile.current.expired &&
		!!profile.current.id
	);
}

async function refresh() {
	if (!canListTunnelConnections()) {
		store.current = {
			connections: undefined,
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
			const connections = await UserService.listTunnelConnections({
				dontLogErrors: true
			});
			if (!canListTunnelConnections() || profile.current.id !== profileID) {
				store.current = {
					connections: undefined,
					loading: false
				};
				return;
			}
			store.current = {
				connections,
				loading: false
			};
		} catch {
			store.current = {
				connections: undefined,
				loading: false
			};
		} finally {
			refreshPromise = undefined;
		}
	})();

	return refreshPromise;
}

function startPolling() {
	pollingConsumers++;

	if (pollingConsumers === 1) {
		void refresh();
		pollingInterval = setInterval(() => void refresh(), POLL_INTERVAL_MS);
	}

	return () => {
		pollingConsumers = Math.max(0, pollingConsumers - 1);
		if (pollingConsumers === 0 && pollingInterval) {
			clearInterval(pollingInterval);
			pollingInterval = undefined;
		}
	};
}

export default store;
