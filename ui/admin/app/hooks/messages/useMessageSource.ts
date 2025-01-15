import { useCallback, useEffect, useState } from "react";

import { ChatEvent } from "~/lib/model/chatEvents";
import { Message, promptMessage, toolCallMessage } from "~/lib/model/messages";

export function useMessageStream(source: Nullish<EventSource>) {
	const [messages, setMessages] = useState<Message[]>([]);
	const [isRunning, setIsRunning] = useState(false);

	const addContent = useCallback((event: ChatEvent) => {
		const {
			content,
			prompt,
			toolCall,
			runComplete,
			input,
			error,
			runID,
			contentID,
			replayComplete,
		} = event;

		setIsRunning(!runComplete && !replayComplete);

		setMessages((prev) => {
			const copy = [...prev];

			// todo(ryanhopperlowe) can be optmized by searching from the end
			const existingIndex = contentID
				? copy.findIndex((m) => m.contentID === contentID)
				: -1;

			if (existingIndex !== -1) {
				const existing = copy[existingIndex];
				copy[existingIndex] = {
					...existing,
					text: existing.text + content,
				};

				return copy;
			}

			if (error) {
				if (error.includes("thread was aborted, cancelling run")) {
					copy.push({
						sender: "agent",
						text: "Message Aborted",
						runId: runID,
						contentID,
						aborted: true,
					});

					return copy;
				}

				copy.push({
					sender: "agent",
					text: error,
					runId: runID,
					error: true,
					contentID,
				});
				return copy;
			}

			if (input) {
				copy.push({
					sender: "user",
					text: input,
					runId: runID,
					contentID,
				});
				return copy;
			}

			if (toolCall) {
				return handleToolCallEvent(copy, event);
			}

			if (prompt) {
				copy.push(promptMessage(prompt, runID));
				return copy;
			}

			if (content) {
				copy.push({
					sender: "agent",
					text: content,
					runId: runID,
					contentID,
				});
				return copy;
			}

			return copy;
		});
	}, []);

	useEffect(() => {
		setMessages([]);

		if (!source) return;

		let replayComplete = false;
		let replayMessages: ChatEvent[] = [];

		source.addEventListener("close", source.close);

		source.addEventListener("message", (chunk) => {
			const event = JSON.parse(chunk.data) as ChatEvent;

			if (event.replayComplete) {
				replayComplete = true;
				replayMessages.forEach(addContent);
				replayMessages = [];
			}

			if (!replayComplete) {
				replayMessages.push(event);
				return;
			}

			addContent(event);
		});

		return () => {
			source.close();
			setIsRunning(false);
		};
	}, [source, addContent]);

	return { messages, isRunning };
}

const findIndexLastPendingToolCall = (messages: Message[]) => {
	for (let i = messages.length - 1; i >= 0; i--) {
		const message = messages[i];
		if (message.tools && !message.tools[0].output) {
			return i;
		}
	}
	return null;
};

const handleToolCallEvent = (messages: Message[], event: ChatEvent) => {
	if (!event.toolCall) return messages;

	const { toolCall } = event;
	if (toolCall.output) {
		const index = findIndexLastPendingToolCall(messages);
		if (index !== null) {
			// update the found pending toolcall message (without output)
			messages[index].tools = [toolCall];
			return messages;
		}
	}

	// otherwise add a new toolcall message
	messages.push(toolCallMessage(toolCall));
	return messages;
};
