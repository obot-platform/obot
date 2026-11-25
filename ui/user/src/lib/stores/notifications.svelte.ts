export type NotificationKind = 'error' | 'success';

export interface Notification {
	id: number;
	kind: NotificationKind;
	message: string;
}

const SUCCESS_DURATION_MS = 5000;

let nextId = 0;

function messageFrom(input: unknown): string {
	if (
		typeof input === 'object' &&
		input !== null &&
		'message' in input &&
		typeof input.message === 'string'
	) {
		return input.message;
	}
	return String(input);
}

function add(input: unknown, kind: NotificationKind = 'error', duration?: number) {
	const id = ++nextId;
	store.items.push({ id, kind, message: messageFrom(input) });
	const timeout = duration ?? (kind === 'success' ? SUCCESS_DURATION_MS : undefined);
	if (timeout) setTimeout(() => remove(id), timeout);
}

function remove(id: number) {
	const index = store.items.findIndex((item) => item.id === id);
	if (index >= 0) store.items.splice(index, 1);
}

const store = $state({
	items: [] as Notification[],
	add,
	remove
});

export default store;
