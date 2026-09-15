const nativeSetTimeout = globalThis.setTimeout;

const MIN_DELAY_TO_CAP_MS = 1000;
const MAX_DELAY_TO_CAP_MS = 15_000;

/** Cap long `setTimeout` delays */
export function useShortTimeouts(maxDelayMs = 1) {
	globalThis.setTimeout = ((handler: TimerHandler, delay?: number, ...args: unknown[]) => {
		const ms = delay ?? 0;
		const next =
			ms >= MIN_DELAY_TO_CAP_MS && ms <= MAX_DELAY_TO_CAP_MS ? Math.min(ms, maxDelayMs) : ms;
		return nativeSetTimeout(handler, next, ...args);
	}) as typeof setTimeout;

	return () => {
		globalThis.setTimeout = nativeSetTimeout;
	};
}
