import { buildMessagesFromProgress } from '$lib/services/chat/messages';
import type { EditorItem } from '$lib/services/editor/index.svelte';
import errors from '$lib/stores/errors.svelte';
import {
	abort as ChatAbort,
	invoke as ChatInvoke,
	runTask as ChatRunTask,
	sendCredentials as ChatSendCredentials
} from './operations';
import { newMessageEventSource } from './operations';
import type { InvokeInput, Messages, Progress, Project, TaskRun } from './types';

export class Thread {
	replayComplete: boolean = false;
	pending: boolean = $state(false);
	threadID?: string;

	readonly #onError: ((error: Error) => void) | undefined;
	#es: EventSource;
	#progresses: Progress[] = [];
	#count: number = 0;
	readonly #project: Project;
	readonly #task?: {
		id: string;
	};
	readonly #runID?: string;
	readonly #onClose?: () => boolean;
	readonly #authenticate?: {
		tools?: string[];
	};
	readonly #items: EditorItem[] = [];

	constructor(
		project: Project,
		opts?: {
			threadID?: string;
			task?: {
				id: string;
			};
			runID?: string;
			authenticate?: {
				tools?: string[];
				local?: boolean;
			};
			onError?: (error: Error) => void;
			// Return true to reconnect, false to close
			onClose?: () => boolean;
			items?: EditorItem[];
		}
	) {
		this.threadID = opts?.threadID;
		this.#project = project;
		this.#task = opts?.task;
		this.#runID = opts?.runID;
		this.#authenticate = opts?.authenticate;
		this.#onError = opts?.onError;
		this.#onClose = opts?.onClose;
		this.#es = this.#reconnect();
		if (opts?.items) {
			this.#items = opts.items;
		}
	}

	#reconnect(): EventSource {
		console.log('Message EventSource initializing', ++this.#count);
		const currentID = this.#count;
		this.replayComplete = false;
		let opened = false;
		const es = newMessageEventSource(this.#project.assistantID, this.#project.id, {
			threadID: this.threadID,
			task: this.#task,
			runID: this.#runID,
			authenticate: this.#authenticate
		});
		es.onmessage = (e) => {
			this.handleMessage(e);
		};
		es.onopen = (e) => {
			console.log('Message EventSource opened', currentID, e);
			opened = true;
		};
		es.addEventListener('reconnect', () => {
			setTimeout(() => {
				console.log('Message EventSource reconnecting', currentID);
				this.#es.close();
				this.#es = this.#reconnect();
			}, 5000);
		});
		es.addEventListener('close', () => {
			console.log('Message EventSource closed by server', currentID);
			if (this.#onClose?.() ?? true) {
				es.dispatchEvent(new Event('reconnect'));
			}
		});
		es.onerror = (e: Event) => {
			if (e.eventPhase === EventSource.CLOSED) {
				console.log('Message EventSource closed', currentID);
				if (opened) {
					opened = false;
				} else {
					console.log('Message EventSource failed to open', currentID);
					es.dispatchEvent(new Event('reconnect'));
				}
			}
		};
		return es;
	}

	async abort() {
		if (!this.threadID) {
			return;
		}
		try {
			await ChatAbort(this.#project.assistantID, this.#project.id, {
				threadID: this.threadID
			});
		} finally {
			this.pending = false;
		}
	}

	async invoke(input: InvokeInput | string) {
		this.pending = true;
		if (this.threadID) {
			await ChatInvoke(this.#project.assistantID, this.#project.id, this.threadID, input);
		}
	}

	async sendCredentials(id: string, response: Record<string, string>) {
		this.pending = true;
		await ChatSendCredentials(id, response);
	}

	// eslint-disable-next-line @typescript-eslint/no-unused-vars
	onMessages(m: Messages) {}

	// eslint-disable-next-line @typescript-eslint/no-unused-vars
	onStepMessages(stepID: string, m: Messages) {}

	#handleSteps() {
		const newMessages = new Map<string, Progress[]>();
		let stepID: string | undefined;
		for (const progress of this.#progresses) {
			if (progress.step?.id) {
				stepID = progress.step?.id.split('{')[0];
				newMessages.delete(stepID);
			}
			if (stepID) {
				if (!newMessages.has(stepID)) {
					newMessages.set(stepID, []);
				}
				newMessages.get(stepID)?.push(progress);
			}
		}

		for (const [stepID, msgs] of newMessages) {
			this.onStepMessages(
				stepID,
				buildMessagesFromProgress(this.#items, msgs, {
					taskID: this.#task?.id,
					runID: this.#runID,
					threadID: this.threadID
				})
			);
		}
	}

	#onProgress(progress: Progress) {
		this.#progresses.push(progress);
		if (this.replayComplete) {
			this.onMessages(
				buildMessagesFromProgress(this.#items, this.#progresses, {
					taskID: this.#task?.id,
					runID: this.#runID,
					threadID: this.threadID
				})
			);
			this.#handleSteps();
		}
	}

	handleMessage(event: MessageEvent) {
		const progress = JSON.parse(event.data) as Progress;
		if (progress.replayComplete) {
			this.replayComplete = true;
		}
		if (progress.threadID) {
			this.threadID = progress.threadID;
		}
		if (progress.error) {
			if (progress.error.includes('abort')) {
				this.#onProgress(progress);
			} else if (this.replayComplete && this.#onError) {
				this.#onError(new Error(progress.error));
			} else if (this.replayComplete) {
				errors.items.push(new Error(progress.error));
			}
		}
		this.#onProgress(progress);
		this.pending = false;
	}

	async runTask(
		taskID: string,
		opts?: {
			stepID?: string;
			input?: string | object;
		}
	): Promise<TaskRun> {
		this.pending = true;
		return await ChatRunTask(this.#project.assistantID, this.#project.id, taskID, opts);
	}

	close() {
		console.log('Thread closing', this.threadID);
		this.#es.close();
	}
}
