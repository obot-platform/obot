import { buildMessagesFromProgress } from '$lib/services/chat/messages';
import errors from '$lib/stores/errors.svelte';
import { abort as ChatAbort, invoke as ChatInvoke, runTask as ChatRunTask } from './operations';
import { newMessageEventSource } from './operations';
import type { InvokeInput, Messages, Progress, TaskRun } from './types';

export class Thread {
	replayComplete: boolean = false;
	pending: boolean = $state(false);
	readonly #onError: ((error: Error) => void) | undefined;
	#es: EventSource;
	readonly #progresses: Progress[] = [];

	constructor(opts?: {
		task?: {
			id: string;
		};
		runID?: string;
		onError?: (error: Error) => void;
		onClose?: () => void;
	}) {
		const reconnect = (): EventSource => {
			console.log('Message EventSource initializing');
			this.replayComplete = false;
			let opened = false;
			const es = newMessageEventSource({
				task: opts?.task,
				runID: opts?.runID
			});
			es.onmessage = (e) => {
				this.handleMessage(e);
			};
			es.onopen = () => {
				console.log('Message EventSource opened');
				opened = true;
			};
			es.addEventListener('reconnect', () => {
				setTimeout(() => {
					console.log('Message EventSource reconnecting');
					opts?.onClose?.();
					es.close();
					this.#es = reconnect();
				}, 5000);
			});
			es.addEventListener('close', () => {
				console.log('Message EventSource closed by server');
				es.dispatchEvent(new Event('reconnect'));
			});
			es.onerror = (e: Event) => {
				if (e.eventPhase === EventSource.CLOSED) {
					console.log('Message EventSource closed');
					if (opened) {
						opened = false;
					} else {
						console.log('Message EventSource failed to open');
						es.dispatchEvent(new Event('reconnect'));
					}
				}
			};
			return es;
		};

		this.#es = reconnect();
		this.#onError = opts?.onError;
	}

	async abort() {
		try {
			await ChatAbort();
		} finally {
			this.pending = false;
		}
	}

	async invoke(input: InvokeInput | string) {
		this.pending = true;
		await ChatInvoke(input);
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
			this.onStepMessages(stepID, buildMessagesFromProgress(msgs));
		}
	}

	#onProgress(progress: Progress) {
		this.#progresses.push(progress);
		if (this.replayComplete) {
			this.onMessages(buildMessagesFromProgress(this.#progresses));
			this.#handleSteps();
		}
	}

	handleMessage(event: MessageEvent) {
		const progress = JSON.parse(event.data) as Progress;
		if (progress.replayComplete) {
			this.replayComplete = true;
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
		return await ChatRunTask(taskID, opts);
	}

	close() {
		if (this.#es.readyState !== EventSource.CLOSED) {
			this.#es.close();
		}
	}
}
