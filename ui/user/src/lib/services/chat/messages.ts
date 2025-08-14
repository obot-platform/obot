import { ABORTED_BY_USER_MESSAGE, ABORTED_THREAD_MESSAGE } from '$lib/constants';
import type { EditorItem } from '$lib/services/editor/index.svelte';
import type { CitationSource, Explain, InputMessage, Message, Messages, Progress } from './types';

type AdditionalOptions = {
	threadID?: string;
	taskID?: string;
	runID?: string;
	onItemsChanged?: (items: EditorItem[]) => void;
	onEditingFile?: (filename: string, content: string) => void;
	onMemoryCall?: () => void;
};

const errorIcon = 'Error';
const assistantIcon = 'Assistant';
const profileIcon = 'Profile';

function toMessageFromInput(s: string): string {
	try {
		const input = JSON.parse(s) as InputMessage;
		if (input.type === 'assistant-prompt') {
			return input.prompt;
		}
	} catch {
		// ignore error
	}
	return s;
}

function setFileContent(
	items: EditorItem[],
	name: string,
	content: string,
	full: boolean = false,
	opts: AdditionalOptions = {}
) {
	const id = opts.runID ? `${opts.taskID}/${opts.runID}/${name}` : `${opts.threadID}/${name}`;

	const existing = items.find((f) => f.id === id);
	if (existing && existing.file) {
		if (full) {
			existing.file.contents = content;
		} else if (content.length < existing.file.contents.length) {
			existing.file.contents = content + existing.file.contents.slice(content.length);
		} else {
			existing.file.contents = content;
		}
	} else {
		items.push({
			id: id,
			name: name,
			file: {
				contents: content,
				buffer: '',
				threadID: opts.threadID,
				taskID: opts.taskID,
				runID: opts.runID
			}
		});
	}

	// select the file
	items.forEach((f) => {
		f.selected = f.name === name;
	});

	if (opts.onItemsChanged) {
		opts.onItemsChanged(items);
	}
}

function reformatInputMessage(msg: Message) {
	try {
		const input = JSON.parse(msg.message.join('')) as {
			prompt?: string;
			explain?: Explain;
			improve?: Explain;
		};
		if (input.prompt) {
			if (input.improve) {
				msg.message = ['Improve: ', ...input.prompt];
			} else {
				msg.message = [input.prompt];
			}
		} else if (input.prompt === '') {
			msg.message = [''];
		}
		if (input.explain) {
			msg.explain = input.explain;
			msg.message = ['Explain'];
		} else if (input.improve) {
			msg.explain = input.improve;
		}
	} catch {
		// ignore error
	}
}

function getFilenameAndContent(msg: Message) {
	let content = msg.message.join('').trim();
	if (msg.toolCall?.input) {
		content = msg.toolCall.input.trim();
	}
	let testContent = content;
	let partial = false;
	let obj = undefined;
	while (testContent) {
		try {
			if (!testContent.endsWith('"}')) {
				partial = true;
				obj = JSON.parse(testContent + '"}');
			} else {
				obj = JSON.parse(testContent);
			}
			break;
		} catch {
			partial = true;
			testContent = testContent.slice(0, -1);
		}
	}
	if (obj) {
		const entries = Object.entries(obj);
		const contentFirst = entries.length > 0 && entries[0][0] === 'content';
		if (contentFirst && partial) {
			// The filename might be incomplete, so remove it
			obj.filename = '';
		}
		return {
			filename: obj.filename,
			content: obj.content
		};
	}
	return {
		filename: '',
		content: ''
	};
}

function reformatWriteMessage(
	items: EditorItem[],
	msg: Message,
	last: boolean,
	opts: AdditionalOptions = {}
) {
	msg.icon = 'Pencil';
	msg.done = !last || msg.toolCall !== undefined;
	msg.sourceName = msg.done ? 'Wrote to Workspace' : 'Writing to Workspace';

	try {
		const obj = getFilenameAndContent(msg);
		if (obj) {
			msg.file = {
				filename: obj.filename,
				content: ''
			};
			msg.file.filename = obj.filename;
			if (obj.content) {
				msg.file.content = obj.content;
			}

			if (last) {
				opts.onEditingFile?.(obj.filename, obj.content);
			}
		}
		msg.message = [];
	} catch {
		// ignore error
	}

	if (last && msg.file?.filename && msg.file?.content) {
		setFileContent(items, msg.file.filename, msg.file.content, msg.toolCall !== undefined, opts);
	}
}

export function buildMessagesFromProgress(
	items: EditorItem[],
	progresses: Progress[],
	opts: AdditionalOptions
): Messages {
	const messages = toMessages(progresses, opts);

	// Post Process for much more better-ness
	messages.messages.forEach((item, i) => {
		if (
			item.tool &&
			(item.sourceName == 'Workspace write' || item.sourceName == 'workspace_write')
		) {
			reformatWriteMessage(items, item, i == messages.messages.length - 1, opts);
			return;
		} else if (item.sent) {
			reformatInputMessage(item);
		}

		// For all but last message
		if (i < messages.messages.length - 1) {
			if (item.oauthURL || item.promptId) {
				item.ignore = true;
			}
			if (item.tool) {
				item.message = [];
			}
		}
	});

	return messages;
}

function toMessages(progresses: Progress[], opts?: AdditionalOptions): Messages {
	let messages: Message[] = [];
	let lastRunID: string | undefined;
	let lastStepID: string | undefined;
	let parentRunID: string | undefined;
	let firstParentRunID: string | undefined;
	let inProgress = false;

	let sources: CitationSource[] = [];

	// Track runIDs present in the current progresses
	const currentRunIDs = new Set<string>();
	for (const progress of progresses) {
		if (progress.runID) {
			currentRunIDs.add(progress.runID);
		}
	}

	for (const [i, progress] of progresses.entries()) {
		if (progress.error) {
			if (progress.runID) {
				for (const message of messages) {
					if (message.runID === progress.runID) {
						message.aborted = true;
					}
				}
			}
		}

		if (progress.step?.id) {
			lastStepID = progress.step.id;
		}

		if (!progress.runID) {
			// if it doesn't have a runID we don't know what do to with it, so ignore
			continue;
		}

		lastRunID = progress.runID;
		inProgress = true;

		if (progress.parentRunID) {
			parentRunID = progress.parentRunID;

			// If this is the first parentRunID we've seen and it doesn't match any current runID
			// AND the parentRunID isn't empty or null
			if (
				!firstParentRunID &&
				!currentRunIDs.has(progress.parentRunID) &&
				progress.parentRunID.trim() !== ''
			) {
				firstParentRunID = progress.parentRunID;
			}
		}

		if (progress.runComplete) {
			lastRunID = progress.runID;
			inProgress = false;
			for (const message of messages) {
				if (message.runID === progress.runID) {
					message.done = true;
				}
			}
		} else {
			inProgress = true;
		}

		if (progress.error) {
			messages.push(newErrorMessage(progress));
		} else if (progress.prompt) {
			if (progress.prompt.metadata?.authType === 'oauth') {
				messages.push(newOAuthMessage(progress));
			}
			if (progress.prompt.fields) {
				messages.push(newPromptAuthMessage(progress));
			}
		} else if (progress.input) {
			// delete the current runID, this is to avoid duplicate messages
			messages = messages.filter((m) => m.runID !== progress.runID);
			messages.push(newInputMessage(progress, parentRunID));
		} else if (progress.content || (progress.waitingOnModel && i == progresses.length - 1)) {
			const found = messages.findLast(
				(m) => m.contentID === progress.contentID && progress.contentID
			);
			if (found) {
				found.message.push(progress.content);
				found.time = new Date(progress.time);
			} else {
				messages.push(newContentMessage(progress, sources.length ? sources : undefined));
				sources = [];
			}
		} else if (progress.toolInput) {
			const found = messages.findLast(
				(m) => m.contentID === progress.contentID && progress.contentID
			);
			if (found) {
				if (progress.toolInput.input) {
					found.message.push(progress.toolInput.input);
				}
			} else {
				const newMessage = newContentMessage(progress);
				messages.push(newMessage);
				if (
					newMessage.sourceName === 'Create Memory' ||
					newMessage.sourceName === 'Update Memory' ||
					newMessage.sourceName === 'Delete Memory'
				) {
					opts?.onMemoryCall?.();
				}
			}
		} else if (progress.toolCall) {
			const found = messages.findLast(
				(m) => m.contentID === progress.contentID && progress.contentID
			);
			if (found) {
				found.toolCall = progress.toolCall;
				found.done = progress.toolCall.output !== undefined;
			} else {
				messages.push(newContentMessage(progress));
			}

			for (const msg of messages) {
				if (msg.runID === progress.runID && msg.toolInput) {
					msg.ignore = true;
				} else if (msg.runID === progress.runID && !msg.toolInput && !msg.toolCall) {
					msg.done = true;
				}
			}

			if (!progress.toolCall.output) continue;

			try {
				const output = JSON.parse(progress.toolCall.output);

				switch (true) {
					case progress.toolCall.name === 'Knowledge':
						sources.push(...(output as CitationSource[]).map((s) => ({ ...s, type: 'Knowledge' })));
						break;
					case output?.results?.length > 0 &&
						'url' in output.results[0] &&
						'content' in output.results[0]:
						sources.push(...(output.results as CitationSource[]));
						break;
				}
			} catch (_) {
				// ignore error
			}
		}

		if (lastStepID && messages.length > 0) {
			messages[messages.length - 1].stepID = lastStepID;
		}
		if (!inProgress && lastStepID) {
			lastStepID = undefined;
		}
	}

	return {
		lastRunID,
		parentRunID: firstParentRunID,
		messages,
		inProgress
	};
}

function newInputMessage(progress: Progress, parentRunID?: string): Message {
	return {
		runID: progress.runID || '',
		parentRunID: parentRunID,
		time: new Date(progress.time),
		icon: profileIcon,
		sourceName: 'You',
		sent: true,
		message: [toMessageFromInput(progress.input || '')],
		done: true,
		username: progress.username
	};
}

function newOAuthMessage(progress: Progress): Message {
	// prompt will not be undefined at this point
	return {
		runID: progress.runID || '',
		time: new Date(progress.time),
		icon: progress.prompt?.metadata?.icon || assistantIcon,
		sourceName: progress.prompt?.name || 'Assistant',
		sourceDescription: progress.prompt?.description,
		oauthURL: progress.prompt?.metadata?.authURL || '',
		message: progress.prompt?.message ? [progress.prompt?.message] : []
	};
}

function newPromptAuthMessage(progress: Progress): Message {
	return {
		runID: progress.runID || '',
		time: new Date(progress.time),
		icon: progress.prompt?.metadata?.icon || assistantIcon,
		sourceName: progress.prompt?.name || 'Assistant',
		sourceDescription: progress.prompt?.description,
		message: progress.prompt?.message ? [progress.prompt?.message] : [],
		fields: progress.prompt?.fields,
		promptId: progress.prompt?.id
	};
}

export const waitingOnModelMessage = 'Thinking really hard...';

function newErrorMessage(progress: Progress): Message {
	let message = progress.error;
	let ignore = false;
	if (message === 'timeout waiting for prompt response from user') {
		message = 'Credentials were not entered within 5 minutes, please try again.';
	} else if (
		message?.toLowerCase().endsWith(ABORTED_THREAD_MESSAGE) ||
		message?.toLowerCase().endsWith(ABORTED_BY_USER_MESSAGE)
	) {
		ignore = true;
	} else if (!message) {
		message = 'Error';
	}
	return {
		time: new Date(progress.time),
		runID: progress.runID || '',
		icon: errorIcon,
		sourceName: 'Assistant',
		message: [message],
		ignore: ignore
	};
}

function newContentMessage(progress: Progress, citations?: CitationSource[]): Message {
	const result: Message = {
		time: new Date(progress.time),
		runID: progress.runID || '',
		icon: assistantIcon,
		sourceName: 'Assistant',
		message: [progress.toolInput?.input ?? progress.content],
		contentID: progress.contentID,
		citations
	};

	if (progress.toolInput) {
		if (progress.toolInput.name) {
			result.sourceName = progress.toolInput.name;
		}
		result.sourceDescription = progress.toolInput.description;
		if (progress.toolInput.metadata?.icon) {
			result.icon = progress.toolInput.metadata.icon;
		}
		result.message = progress.toolInput.input ? [progress.toolInput.input] : [];
		result.toolInput = true;
		result.tool = true;
	}

	if (progress.toolCall) {
		if (progress.toolCall.name) {
			// turn camelCase into words
			let sourceName = progress.toolCall.name.replace(/([a-z])([A-Z])/g, '$1 $2');
			// turn snake_case into words
			sourceName = sourceName.replace(/_/g, ' ');
			// Capitalize first letter
			sourceName = sourceName.charAt(0).toUpperCase() + sourceName.slice(1);
			result.sourceName = sourceName;
		}
		result.sourceDescription = progress.toolCall.description;
		if (progress.toolCall.metadata?.icon) {
			result.icon = progress.toolCall.metadata.icon;
		}
		result.toolCall = progress.toolCall;
		result.tool = true;
		result.done = progress.toolCall.output !== undefined;
	}

	return result;
}
