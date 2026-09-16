import {
	clearSeenTimestamp,
	getSeenTimestamp,
	hasSeenTimestamp,
	markSeenTimestamp
} from './localstate';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const KEY = 'seen-test';
const store = new Map<string, string>();

beforeEach(() => {
	store.clear();
	vi.stubGlobal('localStorage', {
		getItem: (key: string) => store.get(key) ?? null,
		setItem: (key: string, value: string) => {
			store.set(key, value);
		},
		removeItem: (key: string) => {
			store.delete(key);
		}
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('getSeenTimestamp', () => {
	it('returns unseen when the key is missing', () => {
		expect(getSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toEqual({
			seenAt: undefined,
			stale: false
		});
		expect(hasSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toBe(false);
	});

	it('returns unseen for an invalid stored value', () => {
		localStorage.setItem(KEY, 'not-a-date');

		expect(getSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toEqual({
			seenAt: undefined,
			stale: false
		});
		expect(localStorage.getItem(KEY)).toBe('not-a-date');
	});

	it('returns the stored timestamp when the profile is older or missing', () => {
		localStorage.setItem(KEY, '2024-06-01T00:00:00.000Z');

		const { seenAt, stale } = getSeenTimestamp(KEY, '2024-01-01T00:00:00.000Z');
		expect(stale).toBe(false);
		expect(seenAt?.toISOString()).toBe('2024-06-01T00:00:00.000Z');
		expect(hasSeenTimestamp(KEY, '2024-01-01T00:00:00.000Z')).toBe(true);
		expect(hasSeenTimestamp(KEY)).toBe(true);
	});

	it('treats a timestamp as stale when the profile was created after it', () => {
		localStorage.setItem(KEY, '2020-01-01T00:00:00.000Z');

		expect(getSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toEqual({
			seenAt: undefined,
			stale: true
		});
		expect(hasSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toBe(false);
		expect(localStorage.getItem(KEY)).toBeNull();
	});

	it('ignores an invalid profile created value', () => {
		localStorage.setItem(KEY, '2024-06-01T00:00:00.000Z');

		const { seenAt, stale } = getSeenTimestamp(KEY, 'not-a-date');
		expect(stale).toBe(false);
		expect(seenAt?.toISOString()).toBe('2024-06-01T00:00:00.000Z');
	});

	it('parses ISO timestamps that include a timezone offset', () => {
		localStorage.setItem(KEY, '2026-08-04T16:58:40-04:00');

		const { seenAt, stale } = getSeenTimestamp(KEY, '2026-08-04T12:00:00-04:00');
		expect(stale).toBe(false);
		expect(seenAt?.toISOString()).toBe('2026-08-04T20:58:40.000Z');
	});

	it('returns unseen when localStorage is unavailable', () => {
		vi.stubGlobal('localStorage', undefined);

		expect(getSeenTimestamp(KEY, '2026-01-01T00:00:00.000Z')).toEqual({
			seenAt: undefined,
			stale: false
		});
	});

	it('returns unseen when localStorage throws', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => {
				throw new Error('restricted');
			}
		});

		expect(getSeenTimestamp(KEY)).toEqual({ seenAt: undefined, stale: false });
	});
});

describe('markSeenTimestamp and clearSeenTimestamp', () => {
	it('writes an ISO timestamp and can clear it', () => {
		markSeenTimestamp(KEY);
		expect(localStorage.getItem(KEY)).toMatch(/^\d{4}-\d{2}-\d{2}T/);
		expect(hasSeenTimestamp(KEY)).toBe(true);

		clearSeenTimestamp(KEY);
		expect(localStorage.getItem(KEY)).toBeNull();
		expect(hasSeenTimestamp(KEY)).toBe(false);
	});
});
