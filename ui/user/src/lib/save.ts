import { ChatService, type Project } from '$lib/services';

interface FileMonitor extends SaveMonitor {
	onFileChange(id: string, contents: string): void;
}

export function newFileMonitor(
	project: Project,
	opts?: {
		threadID?: string;
	}
): FileMonitor {
	const files: Record<string, string> = {};
	const monitor = newSaveMonitor(() => files, save, commit);

	async function save(newVal: Record<string, string>): Promise<Record<string, string>> {
		for (const [id, contents] of Object.entries(newVal)) {
			const name = id.split('/').pop() || id;
			const f = new File([contents], name, { type: 'text/plain' });
			const projectScoped = id.startsWith('p1') && id.split('/').length === 2;
			const scopedOpts = projectScoped ? { threadID: opts?.threadID } : {};
			await ChatService.saveFile(project.assistantID, project.id, f, scopedOpts);
		}
		return newVal;
	}

	async function commit(newVal: Record<string, string>) {
		for (const [id, contents] of Object.entries(newVal)) {
			if (files[id] === contents) {
				delete files[id];
			}
		}
	}

	function onFileChange(id: string, contents: string) {
		files[id] = contents;
	}

	return {
		start: monitor.start,
		stop: monitor.stop,
		save: monitor.save,
		onFileChange
	};
}

export interface SaveMonitor {
	start(): () => void;
	save(): Promise<void>;
	stop(): void;
}

export function newSaveMonitor<T>(
	getVal: () => T,
	saveFn: (o: T) => Promise<T>,
	// The commit function is used to communicate the saved value in a safe way as to
	// not accidentally rollback the value to a previous state.
	commitFn?: (o: T) => void
): SaveMonitor {
	let saved: string = '';
	let timer: number;

	async function save() {
		const val = getVal();
		const beforeSaved = JSON.stringify(val);
		if (!val || beforeSaved === saved) {
			return;
		}

		const newVal = await saveFn(val);
		const newSaved = JSON.stringify(newVal);
		const afterSaved = JSON.stringify(getVal());
		if (beforeSaved !== afterSaved) {
			return;
		}
		saved = newSaved;
		commitFn?.(newVal);
	}

	function start(): () => void {
		saved = JSON.stringify(getVal());
		timer = setInterval(save, 1000);
		return () => stop();
	}

	function stop() {
		save();
		clearInterval(timer);
	}

	return {
		start,
		save,
		stop
	};
}
