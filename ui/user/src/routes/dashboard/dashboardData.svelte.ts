import { localState } from '$lib/runes/localState.svelte';
import { dashboardLayoutKey, parseDashboardConfig, type DashboardConfig } from './dashboardLayout';

// Rune-based helpers for the dashboard page. Each must be called while the page component
// initializes, because they register effects.

export type RangeDataset<T> = {
	readonly data: T | undefined;
	readonly loading: boolean;
	readonly busy: boolean;
};

export function trackRangeDataset<T>(options: {
	enabled: () => boolean;
	range: () => { start: Date; end: Date };
	fetch: (start: Date, end: Date, signal: AbortSignal) => Promise<T>;
}): RangeDataset<T> {
	let data = $state<T>();
	let loading = $state(true);
	let busy = $state(false);
	let fetchedRange: string | null = null;

	$effect(() => {
		const { start, end } = options.range();
		const rangeKey = `${start.getTime()}-${end.getTime()}`;
		if (!options.enabled()) {
			loading = true;
			return;
		}
		if (fetchedRange === rangeKey) {
			loading = false;
			return;
		}

		const controller = new AbortController();
		loading = true;
		busy = true;
		options
			.fetch(start, end, controller.signal)
			.then((result) => {
				if (controller.signal.aborted) return;
				fetchedRange = rangeKey;
				data = result;
			})
			.catch(() => {
				if (controller.signal.aborted) return;
				fetchedRange = null;
				data = undefined;
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				loading = false;
				busy = false;
			});
		return () => {
			controller.abort();
			busy = false;
		};
	});

	return {
		get data() {
			return data;
		},
		get loading() {
			return loading;
		},
		get busy() {
			return busy;
		}
	};
}

export type Probe = {
	readonly value: boolean;
	readonly settled: boolean;
};

/**
 * Runs a yes/no availability check while `enabled` is true. A successful result is kept for the
 * page visit. A failed one is retried when `retryKey` changes. Requests are aborted when the page
 * is destroyed. Callers should pass dontLogErrors so a failed optional check stays quiet.
 */
export function trackProbe(options: {
	enabled: () => boolean;
	retryKey: () => unknown;
	check: (signal: AbortSignal) => Promise<boolean>;
}): Probe {
	let value = $state(false);
	let settled = $state(false);
	let succeeded = false;
	let inFlight = false;
	const controller = new AbortController();

	$effect(() => {
		options.retryKey();
		if (!options.enabled() || succeeded || inFlight) return;
		inFlight = true;
		options
			.check(controller.signal)
			.then((result) => {
				succeeded = true;
				value = result;
			})
			.catch(() => {
				if (!controller.signal.aborted) value = false;
			})
			.finally(() => {
				inFlight = false;
				if (!controller.signal.aborted) settled = true;
			});
	});

	$effect(() => () => controller.abort());

	return {
		get value() {
			return value;
		},
		get settled() {
			return settled;
		}
	};
}

export type LayoutStore = {
	current: DashboardConfig | null;
	readonly isReady: boolean;
};

export function ownedLayoutStore(currentUserId: () => string | undefined): LayoutStore {
	const owner = currentUserId();
	let memory = $state<DashboardConfig | null>(null);
	const stored = owner
		? localState<DashboardConfig | null>(dashboardLayoutKey(owner), null, {
				parse: parseDashboardConfig
			})
		: undefined;
	const owns = () => !owner || currentUserId() === owner;

	return {
		get current() {
			if (!owns()) return null;
			return stored ? (stored.current ?? null) : memory;
		},
		set current(next: DashboardConfig | null) {
			if (!owns()) return;
			if (stored) stored.current = next;
			else memory = next;
		},
		get isReady() {
			return stored ? stored.isReady : true;
		}
	};
}
