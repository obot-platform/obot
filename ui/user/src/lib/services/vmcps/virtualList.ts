import { VIRTUAL_LIST_OVERSCAN } from './constants';

export type RowRange = { start: number; end: number };

export function averageRowHeight(heights: ReadonlyMap<string, number>, fallback: number): number {
	if (heights.size === 0) return fallback;
	let total = 0;
	for (const height of heights.values()) total += height;
	return total / heights.size;
}

export function buildRowOffsets(
	keys: readonly string[],
	heights: ReadonlyMap<string, number>,
	fallback: number
): Float64Array {
	const offsets = new Float64Array(keys.length + 1);
	for (let i = 0; i < keys.length; i++) {
		offsets[i + 1] = offsets[i] + (heights.get(keys[i]) ?? fallback);
	}
	return offsets;
}

export function rowIndexAt(offsets: ArrayLike<number>, y: number): number {
	const count = offsets.length - 1;
	if (count <= 0) return 0;

	let low = 0;
	let high = count - 1;
	while (low < high) {
		const mid = (low + high + 1) >> 1;
		if (offsets[mid] <= y) {
			low = mid;
		} else {
			high = mid - 1;
		}
	}
	return low;
}

export function visibleRange({
	offsets,
	scrollTop,
	viewportHeight,
	overscan = VIRTUAL_LIST_OVERSCAN
}: {
	offsets: ArrayLike<number>;
	scrollTop: number;
	viewportHeight: number;
	overscan?: number;
}): RowRange {
	const count = offsets.length - 1;
	if (count <= 0) return { start: 0, end: 0 };

	const top = Math.max(0, scrollTop);
	const bottom = top + Math.max(0, viewportHeight);

	const start = Math.max(0, rowIndexAt(offsets, top) - overscan);
	const end = Math.min(count, rowIndexAt(offsets, bottom) + 1 + overscan);
	return { start, end: Math.max(start, end) };
}
