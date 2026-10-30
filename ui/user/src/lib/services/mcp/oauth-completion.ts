const CHANNEL_NAME = 'obot-mcp-oauth-completion';
const MESSAGE_TYPE = 'mcp-oauth-complete';
// Each check can reach the upstream provider, so bursts of announcements collapse into one.
const MIN_CHECK_INTERVAL_MS = 2000;

type CompletionMessage = { type: typeof MESSAGE_TYPE };

function isCompletionMessage(data: unknown): data is CompletionMessage {
	return (
		typeof data === 'object' && data !== null && (data as { type?: unknown }).type === MESSAGE_TYPE
	);
}

// Authenticate links open without an opener, so the callback tab reaches the
// waiting page over a same-origin channel instead of window.opener. The message
// is only a hint to ask the server again; it carries no authentication data.
export function announceOAuthCompletion() {
	if (typeof BroadcastChannel === 'undefined') return;
	const channel = new BroadcastChannel(CHANNEL_NAME);
	channel.postMessage({ type: MESSAGE_TYPE } satisfies CompletionMessage);
	channel.close();
}

export function onOAuthCompletion(
	callback: () => void,
	minIntervalMs = MIN_CHECK_INTERVAL_MS
): () => void {
	if (typeof BroadcastChannel === 'undefined') return () => {};

	// Monotonic, so a wall-clock correction cannot stretch the window and stall later checks.
	let lastRun = -Infinity;
	let trailing: ReturnType<typeof setTimeout> | undefined;

	const run = () => {
		trailing = undefined;
		lastRun = performance.now();
		callback();
	};

	const channel = new BroadcastChannel(CHANNEL_NAME);
	channel.onmessage = (event: MessageEvent) => {
		if (!isCompletionMessage(event.data)) return;
		const wait = lastRun + minIntervalMs - performance.now();
		if (wait <= 0) {
			run();
		} else if (!trailing) {
			// A completion inside the window still gets its own check once the window closes.
			trailing = setTimeout(run, wait);
		}
	};

	return () => {
		clearTimeout(trailing);
		channel.close();
	};
}
