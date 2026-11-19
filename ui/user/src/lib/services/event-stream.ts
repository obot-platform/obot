/** Thrown by readJSONEventStream when an event's data is not valid JSON. */
export class MalformedEventError extends Error {}

/**
 * Reads a server-sent event stream whose events carry JSON data, calling onValue
 * with each parsed value. Only `data:` lines are used; event names, ids and
 * comments are ignored. Throws MalformedEventError for data that isn't JSON, and
 * rethrows anything onValue throws.
 */
export async function readJSONEventStream(
	body: ReadableStream<Uint8Array>,
	onValue: (value: unknown) => void
): Promise<void> {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	let buffered = '';
	let dataLines: string[] = [];

	const consumeLine = (line: string) => {
		if (line === '') {
			if (dataLines.length) {
				let value: unknown;
				try {
					value = JSON.parse(dataLines.join('\n'));
				} catch {
					throw new MalformedEventError('malformed event stream JSON');
				}
				dataLines = [];
				onValue(value);
			}
			return;
		}
		if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
	};

	while (true) {
		const { done, value } = await reader.read();
		buffered += decoder.decode(value, { stream: !done });
		const lines = buffered.split(/\r?\n/);
		buffered = done ? '' : (lines.pop() ?? '');
		for (const line of lines) consumeLine(line);
		if (done) break;
	}
	if (buffered) consumeLine(buffered);
	consumeLine('');
}
