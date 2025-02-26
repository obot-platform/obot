import { createStore } from "zustand";

import {
	ChatEvent,
	GoogleSearchOutput,
	KnowledgeToolOutput,
	TavilySearchOutput,
	ToolCall,
} from "~/lib/model/chatEvents";
import { Message, promptMessage, toolCallMessage } from "~/lib/model/messages";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { handleTry } from "~/lib/utils/handleTry";

type EventInitConfig = {
	onEvent: (event: ChatEvent) => void;
};

export type MessageStore = {
	messages: Message[];
	source: EventSource | null;
	isRunning: boolean;
	cleanupFns: (() => void)[];
	processEvent: (event: ChatEvent, threadId?: string) => void;
	init: (threadId: string, config?: EventInitConfig) => void;
	reset: () => void;
};

export const createMessageStore = () => {
	return createStore<MessageStore>()((set, get) => {
		// pseudo private instance variable
		let parsedSources: { url?: string; content: string }[] = [];

		return {
			messages: [],
			cleanupFns: [],
			source: null,
			isRunning: false,
			processEvent: handleProcessEvent,
			init: handleInit,
			reset: handleReset,
		};

		function handleInit(threadId: string, config?: EventInitConfig) {
			const { onEvent } = config || {};

			const source = ThreadsService.getThreadEventSource(threadId);
			let replayComplete = false;
			let replayMessages: ChatEvent[] = [];

			const handleClose = () => source.close();

			const handleMessage = (chunk: MessageEvent<string>): void => {
				const event = JSON.parse(chunk.data) as ChatEvent;

				onEvent?.(event);

				if (event.replayComplete) {
					replayComplete = true;
					replayMessages.forEach((e) => get().processEvent(e, threadId));
					replayMessages = [];
				}

				if (!replayComplete) {
					replayMessages.push(event);
					return;
				}

				get().processEvent(event, threadId);
			};

			source.addEventListener("close", handleClose);
			source.addEventListener("message", handleMessage);

			const cleanupFns = get().cleanupFns.concat(
				() => source.removeEventListener("close", handleClose),
				() => source.removeEventListener("message", handleMessage)
			);

			set({ cleanupFns, source });
		}

		function handleReset() {
			const { source, cleanupFns: listenerCleanupFns } = get();

			listenerCleanupFns.forEach((fn) => fn());
			source?.close();

			set({
				source: null,
				isRunning: false,
				messages: [],
				cleanupFns: [],
			});
		}

		function handleProcessEvent(event: ChatEvent, threadId?: string) {
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
				time,
			} = event;

			set({ isRunning: !runComplete && !replayComplete });

			set((state) => {
				const copy = [...state.messages];

				const existingIndex = contentID
					? copy.findLastIndex((m) => m.contentID === contentID)
					: -1;

				if (existingIndex !== -1) {
					const existing = copy[existingIndex];
					copy[existingIndex] = {
						...existing,
						text: existing.text + content,
						time: existing.time || time,
					};

					return { messages: copy };
				}

				const messageBase: Partial<Message> = {
					threadId,
					runId: runID,
					contentID,
					time,
				};

				if (error) {
					if (error.includes("thread was aborted, cancelling run")) {
						copy.push({
							...messageBase,
							sender: "agent",
							text: "Message Aborted",
							aborted: true,
						});

						return { messages: copy };
					}

					copy.push({
						...messageBase,
						sender: "agent",
						text: error,
						error: true,
					});
					return { messages: copy };
				}

				if (input) {
					copy.push({
						...messageBase,
						sender: "user",
						text: input,
					});
					return { messages: copy };
				}

				if (toolCall) {
					const toolCallMessage = handleToolCallEvent(copy, event);

					if (toolCallMessage)
						copy.push({ ...messageBase, ...toolCallMessage });

					return { messages: copy };
				}

				if (prompt) {
					copy.push({ ...messageBase, ...promptMessage(prompt) });
					return { messages: copy };
				}

				if (content) {
					copy.push({
						...messageBase,
						sender: "agent",
						text: content,
						knowledgeSources: parsedSources.length ? parsedSources : undefined,
					});

					// reset the knowledge output for the next message
					parsedSources = [];
					return { messages: copy };
				}

				return { messages: copy };
			});
		}

		function handleToolCallEvent(messages: Message[], event: ChatEvent) {
			if (!event.toolCall) return null;

			const { toolCall } = event;

			const sources = pullSources(toolCall);
			if (sources) parsedSources.push(...sources);

			// if the toolCall is an output event
			if (toolCall.output) {
				const index = messages.findLastIndex(
					(m) => m.tools && !m.tools[0].output
				);
				if (index !== -1) {
					// update the previous pending toolcall message (without output)
					messages[index].tools = [toolCall];
					return null;
				}
			}

			// otherwise add a new toolcall message
			return toolCallMessage(toolCall);
		}
	});
};

function pullSources(toolCall: ToolCall) {
	if (!toolCall.output) return;

	const [err, output] = handleTry(() => JSON.parse(toolCall.output));

	if (err) {
		console.error(err);
		return [];
	}

	if (toolCall.name === "Knowledge") {
		const o = output as KnowledgeToolOutput;
		return o;
	}

	if (toolCall.name === "Search") {
		if (toolCall.metadata?.toolBundle === "Tavily Search") {
			const o = output as TavilySearchOutput;

			return o.results;
		}

		const o = output as GoogleSearchOutput;
		return o.results;
	}
}
