import { isValid, parseISO } from 'date-fns';

export type SeenTimestamp = {
	seenAt: Date | undefined;
	stale: boolean;
};

function parseValidDate(value: string | null | undefined): Date | undefined {
	if (!value) return undefined;

	const iso = parseISO(value);
	if (isValid(iso)) return iso;

	const parsed = new Date(value);
	return isValid(parsed) ? parsed : undefined;
}

function withLocalStorage<T>(fallback: T, fn: () => T): T {
	try {
		if (typeof localStorage === 'undefined') return fallback;
		return fn();
	} catch {
		return fallback;
	}
}

export function getSeenTimestamp(key: string, profileCreated?: string | null): SeenTimestamp {
	return withLocalStorage({ seenAt: undefined, stale: false }, () => {
		const seenAt = parseValidDate(localStorage.getItem(key));
		if (!seenAt) return { seenAt: undefined, stale: false };

		const createdAt = parseValidDate(profileCreated);
		const stale = Boolean(createdAt && createdAt.getTime() > seenAt.getTime());
		if (stale) {
			localStorage.removeItem(key);
			return { seenAt: undefined, stale: true };
		}

		return { seenAt, stale: false };
	});
}

export function hasSeenTimestamp(key: string, profileCreated?: string | null): boolean {
	return getSeenTimestamp(key, profileCreated).seenAt !== undefined;
}

export function markSeenTimestamp(key: string) {
	withLocalStorage(undefined, () => {
		localStorage.setItem(key, new Date().toISOString());
	});
}

export function clearSeenTimestamp(key: string) {
	withLocalStorage(undefined, () => {
		localStorage.removeItem(key);
	});
}
