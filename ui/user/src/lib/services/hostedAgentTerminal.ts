/**
 * Framing for the hosted agent terminal websocket.
 *
 * Every message is binary: the first byte selects a channel and the rest is the
 * payload. This mirrors the multiplexing Docker and Kubernetes use for the same
 * job — one ordered connection carrying several kinds of traffic — and needs no
 * length prefix of its own, because a websocket already preserves message
 * boundaries.
 *
 * Keep these values in step with pkg/api/handlers/agentterminal/protocol.go.
 */
export const Channel = {
	/** Keystrokes, browser to sandbox. */
	stdin: 0,
	/** Console output. A TTY session merges all output onto this channel. */
	stdout: 1,
	/**
	 * The sandbox's stderr. Unused for a TTY session, where stderr is already
	 * merged into stdout; reserved for a non-TTY session that keeps them apart.
	 */
	stderr: 2,
	/** JSON messages about the session rather than its content. */
	control: 3
} as const;

export interface ControlMessage {
	type: 'resize' | 'error';
	cols?: number;
	rows?: number;
	/** Present on 'error': why the session failed, in Obot's words. */
	message?: string;
}

export function frame(channel: number, payload: Uint8Array): Uint8Array {
	const message = new Uint8Array(payload.length + 1);
	message[0] = channel;
	message.set(payload, 1);
	return message;
}

/** Returns null for a message with no channel byte, which cannot be attributed. */
export function parseFrame(message: Uint8Array): { channel: number; payload: Uint8Array } | null {
	if (message.length === 0) return null;
	return { channel: message[0], payload: message.subarray(1) };
}

export function controlFrame(message: ControlMessage): Uint8Array {
	return frame(Channel.control, new TextEncoder().encode(JSON.stringify(message)));
}

/**
 * The websocket carries the session cookie, since a browser cannot set an
 * Authorization header on an upgrade. The server only accepts same-origin
 * upgrades, so this URL is deliberately built from the current origin.
 */
export function terminalURL(instanceID: string, cols: number, rows: number): string {
	const url = new URL(
		`/api/hosted-agent-instances/${encodeURIComponent(instanceID)}/terminal`,
		window.location.href
	);
	url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
	url.searchParams.set('cols', String(cols));
	url.searchParams.set('rows', String(rows));
	return url.toString();
}
