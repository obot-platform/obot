/*
    readStream uses a buffer approach to handle streaming data:
    1. It accumulates incoming data in a buffer.
    2. It processes complete messages (separated by '\n\n') as they arrive.
    3. Any incomplete message is kept in the buffer for the next iteration.

    This approach ensures that we don't lose data between chunks and can
    handle messages that might be split across multiple chunks.
*/
export async function readStream<T>({
	reader,
	onChunk,
	onComplete,
}: {
	reader: ReadableStreamDefaultReader<string>;
	onChunk: (data: T) => void;
	onComplete?: (data: T[]) => void;
}) {
	const collected: T[] = [];
	const decoder = new TextDecoder();
	let buffer = "";

	try {
		// eslint-disable-next-line no-constant-condition
		while (true) {
			// Read from the stream
			const { value, done } = await reader.read();
			if (done) break;

			// Decode the chunk and add to buffer
			buffer += decoder.decode(new TextEncoder().encode(value), {
				stream: true,
			});

			// Split buffer into complete messages
			const messages = buffer.split("\n\n");
			// Keep the last (potentially incomplete) message in the buffer
			buffer = messages.pop() || "";

			// Process complete messages
			for (const message of messages) {
				const dataString = message
					.replace(/^id:.*\n/, "")
					.replace(/^data: /, "")
					.trim();
				if (dataString) {
					try {
						const data = JSON.parse(dataString) as T;
						onChunk(data);
						collected.push(data);
					} catch (error) {
						console.error("Error parsing JSON:", error);
					}
				}
			}
		}

		// Process any remaining data in the buffer after stream closes
		if (buffer.trim()) {
			const dataString = buffer.replace(/^data: /, "").trim();
			if (dataString) {
				try {
					const data = JSON.parse(dataString) as T;
					onChunk(data);
					collected.push(data);
				} catch (error) {
					console.error("Error parsing JSON:", error);
				}
			}
		}
	} catch (error) {
		console.error("Error reading stream:", error);
	} finally {
		// Always call onComplete, even if there was an error
		onComplete?.(collected);
	}
}
