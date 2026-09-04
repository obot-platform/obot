import { ESTIMATED_ROW_HEIGHT } from './constants';
import { averageRowHeight, buildRowOffsets, rowIndexAt, visibleRange } from './virtualList';
import { describe, expect, it } from 'vitest';

function keysOf(count: number) {
	return Array.from({ length: count }, (_, i) => `entry-${i}`);
}

function uniformOffsets(count: number, height: number) {
	return buildRowOffsets(keysOf(count), new Map(), height);
}

describe('averageRowHeight', () => {
	it('falls back until something has been measured', () => {
		expect(averageRowHeight(new Map(), ESTIMATED_ROW_HEIGHT)).toBe(ESTIMATED_ROW_HEIGHT);
	});

	it('averages the measured rows', () => {
		const heights = new Map([
			['a', 50],
			['b', 70]
		]);
		expect(averageRowHeight(heights, ESTIMATED_ROW_HEIGHT)).toBe(60);
	});
});

describe('buildRowOffsets', () => {
	it('accumulates measured heights and estimates the rest', () => {
		const offsets = buildRowOffsets(['a', 'b', 'c'], new Map([['b', 30]]), 10);
		expect(Array.from(offsets)).toEqual([0, 10, 40, 50]);
	});

	it('is a single zero for an empty list, so the total height is zero', () => {
		expect(Array.from(buildRowOffsets([], new Map(), 10))).toEqual([0]);
	});
});

describe('rowIndexAt', () => {
	it('finds the row occupying a position', () => {
		const offsets = uniformOffsets(10, 100);
		expect(rowIndexAt(offsets, 0)).toBe(0);
		expect(rowIndexAt(offsets, 99)).toBe(0);
		expect(rowIndexAt(offsets, 100)).toBe(1);
		expect(rowIndexAt(offsets, 250)).toBe(2);
	});

	it('clamps past either end of the list', () => {
		const offsets = uniformOffsets(10, 100);
		expect(rowIndexAt(offsets, -500)).toBe(0);
		expect(rowIndexAt(offsets, 99999)).toBe(9);
		expect(rowIndexAt(uniformOffsets(0, 100), 50)).toBe(0);
	});
});

describe('visibleRange', () => {
	it('covers the viewport plus overscan on both sides', () => {
		expect(
			visibleRange({
				offsets: uniformOffsets(100, 100),
				scrollTop: 1000,
				viewportHeight: 500,
				overscan: 2
			})
		).toEqual({ start: 8, end: 18 });
	});

	it('starts at the top while the list is still below the fold', () => {
		expect(
			visibleRange({
				offsets: uniformOffsets(100, 100),
				scrollTop: -300,
				viewportHeight: 500,
				overscan: 0
			})
		).toEqual({ start: 0, end: 6 });
	});

	it('stops at the last row rather than overrunning the list', () => {
		expect(
			visibleRange({
				offsets: uniformOffsets(20, 100),
				scrollTop: 5000,
				viewportHeight: 500,
				overscan: 5
			})
		).toEqual({ start: 14, end: 20 });
	});

	it('is empty when there is nothing to show', () => {
		expect(
			visibleRange({ offsets: uniformOffsets(0, 100), scrollTop: 0, viewportHeight: 500 })
		).toEqual({ start: 0, end: 0 });
	});

	it('windows a thousand rows down to a screenful', () => {
		const range = visibleRange({
			offsets: uniformOffsets(1000, 68),
			scrollTop: 0,
			viewportHeight: 800
		});
		expect(range.end - range.start).toBeLessThan(30);
	});
});
